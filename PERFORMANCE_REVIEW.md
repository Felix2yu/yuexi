# 月汐 (Yuexi) 性能审查报告

> 审查日期：2026-08-29
> 范围：后端服务（Go / chi / SQLite）+ 前端页面（服务端渲染 HTML + 内嵌 JS + Service Worker）
> 方法论：**静态代码审查**（通读 `internal/db`、`internal/handler`、`internal/service`、`main.go` 及模板/sw）。

---

## ⚠️ 重要声明：本报告基于代码静态分析，非实测数据

仓库内**不存在任何生产监控、pprof 火焰图、慢查询日志、Lighthouse/WebPageTest 实测或压测基线**。因此：

- 所有"现状瓶颈"均为**代码层面的推断**（如算法复杂度、每次请求是否查库、是否阻塞 I/O）。
- 所有"预期收益"均为**工程经验估算**，必须用下方"验证方式"中的手段实测对比后才能确认。
- 在拿到真实监控数据前，请勿把本文的"收益数字"当承诺值。

### 需要补充的上下文（否则部分结论无法定稿）

1. **部署拓扑**：Go 服务前是否已有反向代理 / CDN（nginx、Caddy、Cloudflare 等）？
   - 若已做 gzip，则"缺失 HTTP 压缩"此项应下调优先级（甚至冗余）。
   - 若已有 CDN，则第三方 CDN（Tailwind/字体/Alpine）的"自托管"收益需重新评估。
2. **数据规模**：典型单用户 `records` / `daily_logs` 条数、`persons` 数量、注册用户数。
   - 决定"全量加载/分页"优化的真实收益量级。
3. **部署形态**：单实例还是多实例？SQLite 单写者限制下，多实例无意义。
   - 决定连接池调优与"数据库锁定"风险判断。
4. **目标用户与设备**：是否以移动端为主？浏览器分布？
   - 决定字体子集化、`backdrop-filter` 模糊等前端项的优先级。
5. **第三方通道**：shoutrrr 实际使用的通道类型与超时预期（影响通知检查器的阻塞风险）。
6. **可接受变更窗口**：哪些属于"线上禁止破坏性变更"（如改路由、改 PWA 缓存策略需用户重装 SW）。

---

## 一、后端优化项（按收益从高到低）

### B1. `ServeIcon` 每次请求像素级生成 PNG —— 高收益 / 短期易改
- **问题位置**：`internal/handler/static.go:37-42`（`ServeIcon`）、`51-139`（`generateIcon`）
- **现状瓶颈**：每个图标请求都执行 `generateIcon`——三层嵌套像素循环（背景渐变 + 月牙抗锯齿 + 波浪），`512×512` 约 26 万像素 × 多遍，且 `img.Set` 逐个写。这是**请求路径上的 O(n²) CPU 热点**，无服务端缓存。`/icon-192.png`、`/icon-512.png`、`/favicon.ico` 每次页面加载都会触发。
- **优化方案**：在 `init()` 或首次启动时预生成 32/192/512 三个尺寸的 PNG 字节（`[]byte`），存入包级 `map[int][]byte`；`ServeIcon` 直接 `w.Write` 缓存字节。生成逻辑完全不变，仅缓存结果。
- **预期收益**：消除每请求数十万次像素运算，图标接口的 P99 延迟与 CPU 占用大幅下降（压测下通常 **1~2 个数量级**的 CPU 时间下降）。低风险、无行为变更。
- **实施成本与风险**：低（约 15 行）。风险：无破坏性变更；注意 `init` 阶段失败应 `log.Fatal` 而非 panic 影响启动。
- **验证方式**：① `go test -bench` 对 `generateIcon` 前后对比 ns/op；② 用 `pprof`（`import _ "net/http/pprof"`，`go tool pprof -http`）在 `wrk -t4 -c100 -d30s` 压测下对比 CPU 火焰图，确认 `generateIcon` 从热点消失；③ 对比图标接口 P99。

### B2. 缺少 HTTP 响应压缩 —— 高收益 / 短期易改
- **问题位置**：`main.go:18-19`（`buildRouter` 仅 `Logger` + `Recoverer`，无 `middleware.Compress`）
- **现状瓶颈**：所有 HTML 页面与 JSON API 均以**未压缩明文**传输。模板含大量内联 CSS（layout.html 78-193 行约 4KB+），JSON 含重复字段名，传输体积可观。
- **优化方案**：在 `buildRouter` 中 `r.Use(middleware.Compress(5))`（chi 自带，支持 gzip/brotli 协商）。
- **预期收益**：HTML/JSON 体积通常下降 **70%~90%**，受带宽/延迟限制的客户端（尤其移动端）TTFB 与首屏明显加快。
- **实施成本与风险**：极低（1 行）。**风险/前置条件**：若前面已有 nginx/Cloudflare 做压缩，此项冗余（见"需补充上下文 1"）。不改响应内容，无破坏性。
- **验证方式**：`curl -s -H 'Accept-Encoding: gzip' -I <url>` 对比 `Content-Encoding` 与 `Content-Length`；Lighthouse 的"传输字节数"前后对比。

### B3. SQLite 连接池未配置（默认无限连接）—— 中高收益 / 短期易改
- **问题位置**：`internal/db/db.go:21`（`sql.Open` 后未调用 `SetMaxOpenConns`/`SetMaxIdleConns`/`SetConnMaxLifetime`）
- **现状瓶颈**：`*sql.DB` 默认 `MaxOpenConns=0`（无限）。SQLite 写操作本质串行化，无限连接在高并发下会加剧 `database is locked`（虽有 `busy_timeout(5000)`，但等待与重试成本高）。
- **优化方案**：`db.Init` 中显式配置，例如 `DB.SetMaxOpenConns(4)`、`DB.SetMaxIdleConns(2)`、`DB.SetConnMaxLifetime(time.Hour)`。单写者场景可进一步降到 1（读多写少时 4 较稳妥）。
- **预期收益**：降低高并发下的锁等待与超时错误率，P95 更平稳。
- **实施成本与风险**：低。风险：连接数过小可能限制并发读吞吐，需按压测调参；不改动 SQL 语义。
- **验证方式**：`wrk`/k6 并发压测（写多场景）对比错误率（`database is locked` 计数）与 P95；观察 `DB.Stats()` 的 `WaitCount`/`MaxOpenConnections`。

### B4. 会话每次请求查库（`GetSession`）—— 中收益 / 短期易改
- **问题位置**：`internal/handler/auth.go`（`AuthMiddleware` → `getSession`）→ `internal/db/crud.go:75`（`GetSession` 每次 `QueryRow`）
- **现状瓶颈**：每个受保护请求都至少 1 次 `sessions` 表查询（含潜在的 `DELETE` 过期行写操作）。在登录态高频访问下，会话查询是稳定且可避免的 DB 开销。
- **优化方案**：进程内 TTL 缓存（如 `sync.Map` + 过期时间戳，或 `github.com/patrickmn/go-cache`）。命中则跳过 DB；写操作（登录/登出）时主动失效。注意多实例下缓存不一致——单实例 SQLite 场景无此问题。
- **预期收益**：登录态请求路径减少 1 次 DB 往返，P99 下降，DB QPS 降低。
- **实施成本与风险**：中（需处理过期与失效边界）。风险：缓存与 DB 不一致导致"登出后仍有效/过期判断偏差"——务必在 `CreateSession`/`DeleteSession`/`DeleteUserSessions` 时同步失效缓存。
- **验证方式**：压测对比 QPS 与 P99；断言"登出后立即失效""过期会话被拒"的回归测试仍通过。

### B5. `StatsAPI` 全量扫描 + O(人数×记录数) 内存过滤 —— 中收益 / 短期易改
- **问题位置**：`internal/handler/stats.go:35-50`（`GetRecordsByUser` 取全部记录，再按 `PersonID` 做双重循环过滤）
- **现状瓶颈**：算法复杂度 O(persons × records)；且 `GetDailyLogsByPerson` 在循环内对**每个 person 各查一次 `daily_logs`**（真正的 N+1：人数个 `daily_logs` 查询）。数据量增长时 CPU/DB 线性恶化。
- **优化方案**：① 用 `map[int64][]Record` 一次性按 `person_id` 分组（O(n)）；② 每日日志改为一次 `GetDailyLogsByUser` 后内存分组，或在 SQL 层 `WHERE person_id IN (...)` 批量取。
- **预期收益**：大用户数据下 StatsAPI 耗时与 DB 查询数显著下降。
- **实施成本与风险**：低-中。风险：分组逻辑需保证与现有一致（含 `len(recs)<2` 边界）。
- **验证方式**：构造"10 人 × 每人 200 条记录"基准，对比前后 `time.Now()` 包裹的耗时与 `daily_logs` 查询次数（可在测试里 mock 计数）。

### B6. 记录/日志无分页，全量加载 —— 中收益 / 需重构
- **问题位置**：`internal/db/crud.go:218`（`GetRecordsByUser`）、`455`（`GetAllDailyLogs` 经 `/api/daily` 无 person 时全表返回）
- **现状瓶颈**：一次取回某用户**全部**记录/日志到内存并序列化。数据积累到成千上万条后，内存与序列化开销、以及 JSON 体积都不可控。
- **优化方案**：列表类接口加分页（`LIMIT/OFFSET` 或游标）与时间窗过滤；`/api/daily` 无 `person_id` 时不返回全表（当前设计疑似仅用于管理/调试，应限制或鉴权）。
- **预期收益**：内存峰值与尾延迟随数据量增长保持平稳。
- **实施成本与风险**：中（需改接口契约与前端分页）。风险：改变 API 响应结构属**契约变更**，需前后端协同并保留兼容。
- **验证方式**：注入 1 万条记录的基准库，对比内存 RSS（压测中观察）与首字节延迟；分页后单页耗时稳定。

### B7. 通知检查器的阻塞式外部发送（shoutrrr）—— 低收益 / 短期易改
- **问题位置**：`internal/service/notify.go`（`checkNotifications` 循环内同步 `shoutrrr.Send`）
- **现状瓶颈**：后台 goroutine 每 30 分钟唤醒，对每个启用用户/对象循环做**同步网络发送**（第三方通道可能慢/超时）。虽不在请求路径，但无超时控制时，慢通道会拖长整轮检查并长期占用 DB 连接。
- **优化方案**：为每次发送加 `context.WithTimeout`（如 10s）；单条失败 `continue`，不阻塞其余；检查周期与发送解耦（可并发但有上限）。
- **预期收益**：避免个别慢通道拖垮整轮通知，降低对 DB/连接的影响。
- **实施成本与风险**：低。风险：超时过短可能漏发，需平衡。
- **验证方式**：注入慢/失败 endpoint 的基准，对比单轮检查耗时与成功率。

### B8. 请求级全量日志（`middleware.Logger`）—— 低收益 / 短期易改
- **问题位置**：`main.go:18`（`middleware.Logger`）
- **现状瓶颈**：每个请求写一行日志，高 QPS 下产生可观 I/O。
- **优化方案**：生产环境降级为结构化/采样日志，或仅记录慢请求（自定义 `WrapResponseWriter` 记录 >200ms 的请求）。
- **预期收益**：高 QPS 下降低日志 I/O 与磁盘占用。
- **实施成本与风险**：低。风险：排障信息减少，需保留错误级日志。
- **验证方式**：压测对比吞吐（QPS）与日志写入速率。

---

## 二、前端优化项（按收益从高到低）

### F1. Tailwind Play CDN 同步加载（运行时编译）—— 最高收益 / 需引入构建
- **问题位置**：`internal/handler/template/layout.html:32`（`<script src="https://cdn.tailwindcss.com">`，位于 `<head>`、无 `defer`/`async`）
- **现状瓶颈**：Tailwind Play CDN **不是生产方案**。它在浏览器里实时扫描 DOM、JIT 编译出 CSS，脚本同步加载会**阻塞 HTML 解析与首屏渲染**；且依赖外部网络，断网/慢网时整页无样式。这是首屏/可交互时间最大的单一杀手，并带来约数百 KB 的运行时 JS。
- **优化方案（推荐）**：引入 Tailwind CLI 构建流水线，将用到的类编译成**一份静态 `app.css`** 并自托管（embed 或随二进制发布），`<head>` 内联关键 CSS 或 `<link rel="preload">` 异步加载。`tailwind.config` 中的 `darkMode`/`theme` 直接迁移到 `tailwind.config.js`。
- **预期收益**：消除渲染阻塞脚本，首屏不再依赖外部网络；FCP/TTI 通常可改善 **数百毫秒到数秒**（取决于网络）；移除运行时 JS 开销。
- **实施成本与风险**：中（需加构建步骤、改部署）。**非破坏性**：仅替换样式注入方式，类名不变则视觉一致；建议先 `Content` 扫描确保所有类被收集，避免产物遗漏。
- **验证方式**：Lighthouse（移动端模拟）FCP/TTI/Total Blocking Time 前后对比；WebPageTest 多地点对比；`curl` 确认无外部 tailwindcdn 请求。

### F2. Google Fonts 三字体 + 多字重（含 CJK）渲染阻塞 —— 中高收益 / 短期~重构
- **问题位置**：`layout.html:19-21`（`preconnect` + 同步 `stylesheet` 加载 Cormorant Garamond、Noto Serif SC、Inter 共多个字重）
- **现状瓶颈**：`Noto Serif SC` 是 CJK 大字库，完整字重体积巨大；同步 `<link rel="stylesheet">` 阻塞渲染。`display` 策略未显式声明（默认可能阻塞文本绘制）。
- **优化方案**：① 自托管字体并 `font-display: swap`；② 对 CJK 做**子集化**（仅含用到的字形）或改用系统字体兜底；③ 用 `preload` + `as="font" crossorigin` 异步加载关键字体；④ 精简字重（如仅 400/600）。
- **预期收益**：大幅减少字体下载体积与渲染阻塞时间，中文首屏文本更快可见。
- **实施成本与风险**：中（子集化/自托管需构建）。风险：子集不全会缺字；保留系统字体兜底可兜底。
- **验证方式**：Lighthouse "Eliminate render-blocking resources"、字体传输字节数、FCP 对比；DevTools Network 字体 waterfall。

### F3. Service Worker 对动态页面 cache-first —— 中收益 / 短期易改
- **问题位置**：`internal/handler/static/sw.js:2-9`（`STATIC_ASSETS` 含 `/`、`/settings`、`/person`、`/export` 等动态 HTML）+ `40-56`（页面走 cache-first）
- **现状瓶颈**：这些页面是**带实时数据的动态页**，却用 cache-first 策略——用户新增记录后返回这些页面可能看到**过期内容**（需等下次 fetch 才更新，且先serve旧缓存）。既是正确性问题，也影响感知性能。
- **优化方案**：对 HTML navigation 请求改用 **network-first（失败回退缓存）**；仅静态资源（图标、manifest、CDN 脚本）保留 cache-first。
- **预期收益**：保证数据时效性，消除"看到旧页面"的困惑；网络良好时反而更快（直连）。
- **实施成本与风险**：低。**注意**：改 SW 策略后旧版 SW 可能仍生效，需 bump `CACHE_NAME`（如 `yuexi-v2`）并触发 `activate` 清理——属 PWA 常规升级，无破坏性。
- **验证方式**：创建记录 → 导航回 `/` → 断言页面含新数据；离线时仍能看到上一版本（回退）。

### F4. 昂贵的 CSS 合成（`backdrop-filter` 模糊 + 固定模糊 blob）—— 中收益 / 短期易改
- **问题位置**：`layout.html:100-101`（`.glass-card { backdrop-filter: blur(20px) }`）、`185-192`（`.decorative-blob { filter: blur(80px) }`，且 `position: fixed`）
- **现状瓶颈**：`backdrop-filter` 与大面积 `filter: blur` 触发昂贵的合成层与重绘，移动端/Safari 上滚动与交互易掉帧；`position: fixed` 模糊元素还会在滚动时持续重绘。
- **优化方案**：移动端降低/移除 `blur` 半径；用半透明纯色替代 `backdrop-filter`；`will-change` 慎用；blob 改为静态背景图或降低模糊层数量。
- **预期收益**：提升滚动/交互流畅度，减少长任务（Long Tasks），改善 INP。
- **实施成本与风险**：低。风险：视觉微调，需设计确认观感。
- **验证方式**：DevTools Performance 录制滚动，对比长任务数量与帧率（目标 ≥60fps）；Lighthouse INP 指标前后对比。

### F5. Alpine.js 等第三方脚本走外部 CDN —— 低中收益 / 短期~重构
- **问题位置**：`layout.html:77`（`https://cdn.jsdelivr.net/npm/alpinejs@3.x.x/dist/cdn.min.js`，`defer`）
- **现状瓶颈**：依赖外部 CDN 可用性，增加了一跳 RTT；`3.x.x` 浮点版本每次解析到最新小版本，存在缓存抖动。
- **优化方案**：与 F1/F2 一并纳入构建，自托管 `alpine.min.js`（注意 Alpine 需在使用前 `defer` 且其 `alpine:init` 时序）。锁定具体版本号以获得稳定缓存。
- **预期收益**：减少外部依赖与 RTT，CDN 命中更稳定。
- **实施成本与风险**：低-中（需构建整合）。风险：Alpine 初始化时序需验证。
- **验证方式**：Network 面板确认无外部 CDN 请求；Lighthouse 第三方阻塞对比。

### F6. 首屏资源未充分利用长效缓存头 —— 低收益 / 短期易改
- **问题位置**：`static.go:24-35`（`manifest.json` 无 `Cache-Control`；`sw.js` 已 `no-cache` 合理；图标已 `max-age=86400` 合理）
- **现状瓶颈**：`manifest.json` 缺失缓存头，每次重新校验；整体静态资源缓存策略可统一。
- **优化方案**：为 `manifest.json` 加 `Cache-Control: public, max-age=86400`；统一静态响应头管理。
- **预期收益**：重复访问时减少请求与校验，微小但稳。
- **实施成本与风险**：极低。风险：无。
- **验证方式**：`curl -I` 确认响应头；重复访问 Network 命中 `(disk cache)`。

---

## 三、综合优先级总表

| 优先级 | 编号 | 维度 | 类型 | 项 | 关键验证指标 |
|---|---|---|---|---|---|
| 1 | F1 | 前端 | 需构建 | Tailwind Play CDN 同步加载 | Lighthouse FCP/TTI |
| 2 | B1 | 后端 | 短期 | ServeIcon 每请求生成 PNG | pprof CPU / 接口 P99 |
| 3 | B2 | 后端 | 短期 | 缺失 HTTP 压缩 | 传输字节 / TTFB |
| 4 | F2 | 前端 | 重构 | Google Fonts 大字体阻塞 | 字体体积 / FCP |
| 5 | B3 | 后端 | 短期 | SQLite 连接池未配置 | 压测错误率 / P95 |
| 6 | B4 | 后端 | 短期 | 会话每请求查库 | QPS / P99 |
| 7 | F3 | 前端 | 短期 | SW 动态页 cache-first | 数据时效性回归 |
| 8 | B5 | 后端 | 短期 | StatsAPI N+1 与 O(n²) | 基准耗时 / 查询数 |
| 9 | F4 | 前端 | 短期 | 昂贵 blur 合成 | INP / 帧率 |
| 10 | B6 | 后端 | 重构 | 列表无分页全量加载 | 内存 RSS / 尾延迟 |
| 11 | F5 | 前端 | 重构 | 第三方 CDN 脚本自托管 | 外部请求数 |
| 12 | B7 | 后端 | 短期 | 通知发送无超时 | 单轮耗时 |
| 13 | F6 | 前端 | 短期 | 静态头不统一 | 缓存命中 |
| 14 | B8 | 后端 | 短期 | 全量请求日志 | 吞吐 |

**短期易改项（建议第一批，均非破坏性）**：B1、B2、B3、B4、B5、B7、B8、F3、F4、F6。
**需重构/引入构建项（第二批，需评估窗口）**：F1、F2、B6、F5。

---

## 四、验证方法论附录（落地前必做）

- **后端 CPU 热点**：`import _ "net/http/pprof"` + `go tool pprof -http=:8081 http://localhost:8080/debug/pprof/profile`，配合 `wrk -t4 -c200 -d30s` 压测。
- **后端延迟/吞吐**：`wrk` 或 `k6` 对比改造前后 P50/P95/P99 与 QPS；`DB.Stats()` 观察 `WaitCount`/`InUse`。
- **慢查询/索引**：对关键查询执行 `EXPLAIN QUERY PLAN SELECT ...`，确认命中 `idx_records_person_id` 等索引，无全表扫描。
- **前端加载**：Lighthouse（移动端模拟，throttling）看 FCP/LCP/TTI/TBT/INP；WebPageTest 多地点；DevTools Performance 看长任务与帧率；Network 看请求数、传输字节、缓存命中。
- **AB 对比纪律**：每项改造单独提交、单独压测，避免多变量混淆；保留改造前基线快照（Lighthouse JSON、pprof、wrk 报告）。

> 结论：当前架构（SQLite + 服务端渲染）对单用户/小规模场景性能充足，主要瓶颈集中在 **①前端外部 CDN 运行时依赖（Tailwind/字体/Alpine）**、**②后端请求路径上的重复/重 CPU 工作（图标生成、每请求查库、无压缩）**。优先实施"短期易改项"即可在不引入破坏性变更的前提下获得明显收益；"需重构项"建议结合部署形态（是否有前置压缩/CDN）再排期。

---

## 五、第一批短期易改项实施记录（2026-08-29）

部署拓扑已确认：**Docker 容器 + nginx 反向代理前置**。据此对原方案做两处取舍调整：
- **B2 压缩保留**：nginx 默认通常仅 gzip `text/html`，JSON API 多未被压缩，故 Go 侧 `middleware.Compress(5)` 对 API 仍有价值；且响应已带 `Content-Encoding` 时 nginx 透传、不会重复压缩。
- **B8 请求日志降级保留**：nginx 已记录全量 access log，Go 侧改为仅记录慢请求（>200ms）与 5xx，降低应用层 I/O 属合理收敛。

### 已落地项（均非破坏性，单独提交）

| 编号 | 改动文件 | 实施内容 |
|---|---|---|
| B1 | `internal/handler/static.go` | 预生成 PNG 字节缓存（`iconCache sync.Map`），图标请求不再每请求像素级重算 |
| F6 | `internal/handler/static.go` | `manifest.json` 增加 `Cache-Control: public, max-age=86400` |
| B3 | `internal/db/db.go` | `Init` 中配置 `SetMaxOpenConns(4)` / `SetMaxIdleConns(2)` / `SetConnMaxLifetime(0)` |
| B4 | `internal/db/crud.go` | `GetSession` 加进程内 `sync.Map` 缓存；`CreateSession`/`DeleteSession`/`DeleteExpiredSessions`/`DeleteUserSessions` 同步失效 |
| B5 | `internal/handler/stats.go` + `internal/db/crud.go` | `StatsAPI` 改两次 O(n) 分组，消除 O(人数×记录) 与每日日志 N+1；新增 `db.GetDailyLogsByUser` |
| B7 | `internal/service/notify.go` | `sendNotification` 加 10s 超时（`select` + goroutine），避免慢通道拖垮整轮检查 |
| B2 | `main.go` | `buildRouter` 增加 `middleware.Compress(5)` |
| B8 | `main.go` | 以 `slowRequestLogger`（仅记 >200ms / 5xx）替换 `middleware.Logger` |
| F3 | `internal/handler/static/sw.js` | 动态导航请求改 network-first（保证数据时效），静态资源保持 cache-first，`CACHE_NAME` 升 `yuexi-v2` |
| F4 | `internal/handler/template/layout.html` | `decorative-blob` 提升为独立合成层（`will-change: transform` + `translateZ(0)`），减少滚动重绘 |

### 验证结果
- `go build ./...`、`go vet ./...`、`go test -race ./...` 全部通过。
- 覆盖率：main 91.8%、db 78.4%、handler 87.1%、service 65.6%（db 因新增 `GetDailyLogsByUser`/连接池/缓存分支略有下降，整体仍约 84%）。
- 回归测试 `TestExpiredSession`/`TestSessionCRUD`/`TestGetSession` 等验证了缓存失效与过期判定仍正确。

### 未做（留待第二批/需评估）
- F1（Tailwind 构建化）、F2（字体子集化/自托管）、F5（Alpine 自托管）、B6（列表分页）属"需重构/引入构建"项，结合 nginx 是否已有 CDN/压缩再排期。
- 所有"预期收益"数字仍需按报告第四节方法论做实测对比（pprof / wrk / Lighthouse），本报告不承诺量化结果。
