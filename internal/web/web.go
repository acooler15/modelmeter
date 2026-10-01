// Package web 托管前端构建产物(web/dist):静态资源直接服务,
// 未命中的路径回退到 index.html 以支持 vue-router 的 history 模式。
package web

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	modelmeter "github.com/acooler15/modelmeter"
)

// 嵌入 FS(embed.FS)没有有效的 Last-Modified/ModTime,协商缓存
// (304 协商)不可用,只能靠显式 Cache-Control 表达缓存策略:
//   - 入口页(index.html 与 SPA 回退)用 no-cache:入口页承载对
//     hash 命名 JS/CSS 的引用,升级后若浏览器仍用旧入口页,就会
//     继续加载旧 JS,表现为"新功能未生效";no-cache 强制每次
//     普通刷新都重新校验入口页,从服务端根治该问题。
//   - assets/ 下文件用 immutable 长缓存:Vite 产物文件名含内容
//     hash,内容变化即换名,长缓存不会读到过期内容。
const (
	cacheControlEntry  = "no-cache"
	cacheControlAssets = "public, max-age=31536000, immutable"
)

// Register 把前端静态资源与 SPA 回退挂到路由上。
// dist 未构建(仅有占位文件)时,页面路由返回友好的中文提示。
func Register(r *gin.Engine) {
	sub, err := fs.Sub(modelmeter.DistFS, "web/dist")
	if err != nil {
		// 嵌入目录由编译期 //go:embed 保证存在,此处仅防御性兜底
		panic(err)
	}

	fileServer := http.FileServer(http.FS(sub))
	// index.html 很小,启动时读入内存,回退时零磁盘开销
	indexBytes, indexErr := fs.ReadFile(sub, "index.html")

	// serveIndex 以 no-cache 返回 SPA 入口页;dist 未构建时保持
	// 原有的占位提示语义(提示响应是给开发者的临时输出,无需缓存策略)。
	serveIndex := func(c *gin.Context) {
		if indexErr != nil {
			c.String(http.StatusServiceUnavailable,
				"前端尚未构建:请先在 web/ 目录执行 npm install && npm run build")
			return
		}
		c.Header("Cache-Control", cacheControlEntry)
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexBytes)
	}

	r.NoRoute(func(c *gin.Context) {
		p := strings.TrimPrefix(c.Request.URL.Path, "/")

		// 直接请求 /index.html 与回退同源同缓存策略:它命中静态文件
		// 分支、由 fileServer 返回时不带任何缓存头,必须单独拦截,
		// 否则浏览器会启发式缓存入口页,升级后仍加载旧 JS。
		if p == "index.html" {
			serveIndex(c)
			return
		}

		// 先尝试按静态文件服务(如 /assets/index-xxx.js)
		if p != "" {
			if f, err := sub.Open(p); err == nil {
				if st, statErr := f.Stat(); statErr == nil && !st.IsDir() {
					f.Close()
					if strings.HasPrefix(p, "assets/") {
						// 文件名含内容 hash,长缓存安全;其余文件
						// (如未来的根级 favicon)保持无头现状
						c.Header("Cache-Control", cacheControlAssets)
					}
					fileServer.ServeHTTP(c.Writer, c.Request)
					return
				}
				f.Close()
			}
		}

		// 其余路径一律回退到 SPA 入口,由前端路由接管
		serveIndex(c)
	})
}
