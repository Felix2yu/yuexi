# 月汐 (Yuexi)

[![codecov](https://codecov.io/gh/Felix2yu/yuexi/branch/main/graph/badge.svg)](https://codecov.io/gh/Felix2yu/yuexi)
[![CI](https://github.com/Felix2yu/yuexi/actions/workflows/ci.yml/badge.svg)](https://github.com/Felix2yu/yuexi/actions/workflows/ci.yml)

> 一个演出 / 观演记录与周期追踪的 Web 应用。

## 功能特性

- **多用户账户**：注册、登录、登出、修改密码。
- **记录对象管理**：维护多个「人物 / 演出主体」档案。
- **记录 CRUD**：演出与周期记录的创建、编辑、删除。
- **预测与检测**：周期与排卵期预测、异常周期检测。
- **统计看板**：可视化统计与每日日志记录。
- **通知系统**：通知配置、连接测试与后台定时检查。
- **数据可移植**：一键导出 / 导入，便于备份与迁移。
- **设置中心**：账户与偏好设置。
- **PWA 支持**：可安装、带 Service Worker 与图标。
- **健康检查**：`/health` 端点，便于容器探针。

## 技术栈

- **语言**：Go 1.27
- **路由**：[go-chi/chi](https://github.com/go-chi/chi) v5
- **数据库**：SQLite（[modernc.org/sqlite](https://modernc.org/sqlite)，纯 Go 实现，无需 CGO）
- **视图**：内嵌 HTML 模板（`embed`）
- **后台任务**：通知定时检查器

## 快速开始

### 本地运行

```bash
# 需要 Go 1.27+
go build -o yuexi .
./yuexi
# 默认监听 http://localhost:8080
```

### 使用 Docker

```bash
docker build -t yuexi .
docker run -d -p 8080:8080 -v "$(pwd)/data:/app/data" yuexi
```

## 配置

通过环境变量进行配置：

| 变量             | 说明                     | 默认值         |
| ---------------- | ------------------------ | -------------- |
| `YUEXI_PORT`     | HTTP 监听端口            | `8080`         |
| `YUEXI_DB_PATH`  | SQLite 数据库文件路径    | `data/yuexi.db` |

## 路由概览

### 认证（无需登录）

| 方法 | 路径          |
| ---- | ------------- |
| GET  | `/login`      |
| POST | `/login`      |
| GET  | `/register`   |
| POST | `/register`   |
| POST | `/logout`     |

### 受保护路由（需登录）

| 方法            | 路径                          | 说明             |
| --------------- | ----------------------------- | ---------------- |
| GET             | `/`                           | 首页             |
| GET             | `/person`                     | 对象列表         |
| POST            | `/person/create`              | 新建对象         |
| GET / POST      | `/person/edit`                | 编辑对象         |
| POST            | `/person/delete`              | 删除对象         |
| GET             | `/settings`                   | 设置页           |
| GET / POST      | `/settings/password`          | 修改密码         |
| POST            | `/record/create`              | 新建记录         |
| POST            | `/record/edit`                | 编辑记录         |
| POST            | `/record/delete`              | 删除记录         |
| GET             | `/api/records`                | 记录列表 API     |
| GET             | `/export`                     | 导出页           |
| GET             | `/export/download`            | 下载导出文件     |
| POST            | `/import`                     | 导入数据         |
| GET / POST      | `/api/notification`           | 通知配置         |
| POST            | `/api/notification/test`      | 通知连接测试     |
| GET             | `/api/notification/status`    | 通知状态         |
| GET             | `/api/anomaly`                | 周期异常检测     |
| GET             | `/stats`                      | 统计页           |
| GET             | `/api/stats`                  | 统计 API         |
| GET / POST / DELETE | `/api/daily`              | 每日日志         |

### 系统与 PWA

| 方法 | 路径              | 说明            |
| ---- | ----------------- | --------------- |
| GET  | `/health`         | 健康检查        |
| GET  | `/manifest.json`  | PWA 清单        |
| GET  | `/sw.js`          | Service Worker  |
| GET  | `/icon-192.png`   | 图标 (192px)    |
| GET  | `/icon-512.png`   | 图标 (512px)    |
| GET  | `/icon-32.png`    | 图标 (32px)     |
| GET  | `/favicon.ico`    | 站点图标        |

## 测试与覆盖率

```bash
# 运行全部测试（含竞态检测与覆盖率）
go test -race -coverprofile=coverage.out -covermode=atomic ./...

# 查看逐函数覆盖率
go tool cover -func=coverage.out
```

CI 通过 GitHub Actions 在每次 push / PR 自动执行 `go build` → `go vet` → `go test`，并将覆盖率上传至 [Codecov](https://codecov.io/gh/Felix2yu/yuexi)。

## 项目结构

```
.
├── main.go                 # 程序入口与路由装配（buildRouter）
├── internal/
│   ├── db/                 # 存储层：SQLite 初始化、模型与 CRUD
│   ├── service/            # 业务逻辑：周期计算、导出导入、通知检查
│   └── handler/            # HTTP 处理器：页面与 API
├── Dockerfile              # 多阶段构建（CGO_ENABLED=0）
├── codecov.yml             # Codecov 状态检查配置
└── .github/workflows/ci.yml
```

## License

详见仓库 `LICENSE` 文件（若未提供，默认保留所有权利）。
