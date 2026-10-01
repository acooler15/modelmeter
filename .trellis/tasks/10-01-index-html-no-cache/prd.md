# 嵌入式前端入口页缓存头修复

## Goal

修复"升级后浏览器仍显示旧页面"的问题:嵌入式前端服务的入口页(index.html
及 SPA 回退)响应必须带 `Cache-Control: no-cache`,内容 hash 化的 assets
静态文件带 immutable 长缓存,保证每次发版后浏览器普通刷新即可加载新版本。

## 背景

用户升级到"接口配置 API Key 明文显示"版本后,浏览器因缓存旧 index.html
继续加载旧 JS(其表格列绑定已被删除的 `api_key_masked` 字段),API Key 列
显示为空,用户误以为功能未生效,被迫手动强制刷新。

实测(`curl -sI http://localhost:8422/`):`internal/web/web.go` 对
index.html 与静态文件响应均未设置任何缓存头,浏览器按启发式策略缓存入口页。
该问题会在每次升级时反复出现,必须从服务端根治。

## Requirements

1. `internal/web/web.go`:
   - SPA 回退响应(NoRoute 走 `c.Data` 的 index.html 分支)加
     `Cache-Control: no-cache`。
   - **直接请求 `/index.html`** 的响应同样加 `Cache-Control: no-cache`
     (注意:该路径目前命中静态文件分支、由 fileServer 返回,不走回退,
     需单独处理;入口页字节已读入内存,可直接用 `c.Data` 返回)。
   - `assets/` 前缀的静态文件响应加
     `Cache-Control: public, max-age=31536000, immutable`
     (Vite 产物文件名含内容 hash,长缓存安全)。
   - 其余行为不变:404 未命中回退 SPA、前端未构建时的占位提示、
     非 assets 文件的处理。
2. 测试:`internal/web` 包用 `httptest` 补充/扩展用例,至少覆盖三类断言:
   SPA 回退(含深层路径回退)响应头、直接请求 `/index.html` 响应头、
   `assets/` 文件响应头;沿用表驱动风格与中文注释。
3. 规范沉淀(收尾阶段):`.trellis/spec/backend/quality-guidelines.md`
   增加嵌入式 SPA 缓存头契约条目——嵌入 FS 无 Last-Modified/ETag,协商
   缓存不可用,入口页必须 no-cache,assets 依赖文件名 hash 可 immutable
   长缓存。

## Acceptance Criteria

- [ ] `curl -I http://localhost:PORT/` 与 `/index.html` 响应均含
      `Cache-Control: no-cache`。
- [ ] `curl -I http://localhost:PORT/assets/<任一js>` 响应含
      `Cache-Control: public, max-age=31536000, immutable`。
- [ ] `go test ./internal/... ./cmd/...` 全绿;`go build ./...` 通过。
- [ ] 重新构建二进制后,浏览器普通刷新(非强制刷新)即可加载新版页面。
- [ ] 规范条目已同步。

## Out of Scope

- ETag / Last-Modified 协商缓存(嵌入 FS 无修改时间,不适用)。
- 前端构建配置(Vite)改动。
