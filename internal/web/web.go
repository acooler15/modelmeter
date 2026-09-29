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

	r.NoRoute(func(c *gin.Context) {
		// 先尝试按静态文件服务(如 /assets/index-xxx.js)
		p := strings.TrimPrefix(c.Request.URL.Path, "/")
		if p != "" {
			if f, err := sub.Open(p); err == nil {
				if st, statErr := f.Stat(); statErr == nil && !st.IsDir() {
					f.Close()
					fileServer.ServeHTTP(c.Writer, c.Request)
					return
				}
				f.Close()
			}
		}

		// 其余路径一律回退到 SPA 入口,由前端路由接管
		if indexErr != nil {
			c.String(http.StatusServiceUnavailable,
				"前端尚未构建:请先在 web/ 目录执行 npm install && npm run build")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexBytes)
	})
}
