/*
 * 月汐 Service Worker —— 依据 pwa-preset/sw-template.js（契约见 ../../PWA规范.md）
 *
 * 版本号由 Go 在 /sw.js 下发时把 __BUILD_VERSION__ 替换为内嵌静态资源的内容哈希
 * （见 internal/handler/static.go 的 swVersion）。前端资产一变，缓存名即变，
 * activate 阶段就会淘汰旧缓存 —— 不再像过去那样把 `yuexi-v3` 写死。
 */
const BUILD_VERSION = '__BUILD_VERSION__';
const CACHE_PREFIX = 'yuexi';
const CACHE_VERSION = `${CACHE_PREFIX}-${BUILD_VERSION}`;
const MAX_ENTRIES = 200;

// 离线可用的页面壳（这些路由需要登录态，故安装时逐个缓存、单个失败不阻断整体安装）
const SHELL_ASSETS = ['/', '/settings', '/person', '/export'];

self.addEventListener('install', event => {
  event.waitUntil(
    caches.open(CACHE_VERSION).then(cache =>
      Promise.all(
        SHELL_ASSETS.map(url => cache.add(url).catch(() => undefined))
      )
    )
  );
  self.skipWaiting();
});

self.addEventListener('activate', event => {
  event.waitUntil(
    caches.keys().then(keys =>
      Promise.all(
        keys
          .filter(k => k.startsWith(`${CACHE_PREFIX}-`) && k !== CACHE_VERSION)
          .map(k => caches.delete(k))
      )
    )
  );
  self.clients.claim();
});

self.addEventListener('message', event => {
  if (event.data === 'SKIP_WAITING') self.skipWaiting();
});

/** 写缓存并裁剪到上限（FIFO）。 */
async function putWithLimit(request, response) {
  const cache = await caches.open(CACHE_VERSION);
  await cache.put(request, response);
  const keys = await cache.keys();
  if (keys.length > MAX_ENTRIES) {
    for (const k of keys.slice(0, keys.length - MAX_ENTRIES)) {
      await cache.delete(k);
    }
  }
}

self.addEventListener('fetch', event => {
  const { request } = event;
  const url = new URL(request.url);

  if (request.method !== 'GET') return;
  if (url.origin !== self.location.origin) return;

  // API 与记录变更：网络优先，失败回退缓存（带条目上限，避免无限膨胀）
  if (url.pathname.startsWith('/api/') || url.pathname.startsWith('/record/')) {
    event.respondWith(
      fetch(request)
        .then(response => {
          if (response.ok) putWithLimit(request, response.clone());
          return response;
        })
        .catch(async () => (await caches.match(request)) || Response.error())
    );
    return;
  }

  // HTML 导航（服务端渲染、带数据）：网络优先，保证看到最新数据；离线回落缓存或首页壳
  if (request.mode === 'navigate') {
    event.respondWith(
      fetch(request)
        .then(response => {
          if (response.ok) putWithLimit(request, response.clone());
          return response;
        })
        .catch(async () => (await caches.match(request)) || caches.match('/'))
    );
    return;
  }

  // 静态资源：缓存优先，未命中再走网络
  event.respondWith(
    caches.match(request).then(
      cached =>
        cached ||
        fetch(request).then(response => {
          if (response.ok) putWithLimit(request, response.clone());
          return response;
        })
    )
  );
});
