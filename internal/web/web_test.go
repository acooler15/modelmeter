package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	modelmeter "github.com/acooler15/modelmeter"
)

// newTestRouter 构造仅注册 web 托管的最小路由,与 main.go 的挂载方式
// 一致:未命中路径全部走 NoRoute 回退。
func newTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	Register(r)
	return r
}

// doGet 执行 GET 请求并返回响应记录器,便于同时断言状态码、响应头与响应体。
func doGet(t *testing.T, r *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

// TestRegister_缓存头 锁定嵌入式 SPA 的缓存契约:入口页(回退与直接
// 请求)一律 no-cache,assets/ 哈希产物 immutable 长缓存;同时确认
// 未命中回退到 index.html 的原有语义不回归。
func TestRegister_缓存头(t *testing.T) {
	// 契约建立在真实构建产物上;dist 未构建(仅占位文件)时无产物
	// 可验,跳过而非误报失败
	if _, err := fs.ReadFile(modelmeter.DistFS, "web/dist/index.html"); err != nil {
		t.Skip("web/dist 尚未构建,缓存头契约仅在构建产物上验证")
	}
	wantIndex, err := fs.ReadFile(modelmeter.DistFS, "web/dist/index.html")
	if err != nil {
		t.Fatalf("读取嵌入 index.html 失败: %v", err)
	}

	r := newTestRouter(t)

	// 入口页与回退:四个入口(根路径、深层路径、直接请求 /index.html、
	// assets 未命中)都应返回同一份 index.html 且带 no-cache——任何
	// 一个入口被浏览器启发式缓存都会导致升级后加载旧 JS
	entryCases := []struct {
		name string
		path string
	}{
		{"根路径回退", "/"},
		{"深层路径回退", "/providers"},
		{"直接请求入口页", "/index.html"},
		{"assets未命中回退", "/assets/not-exist.js"},
		{"assets目录请求回退", "/assets/"},
	}
	for _, tc := range entryCases {
		t.Run("入口页no-cache_"+tc.name, func(t *testing.T) {
			w := doGet(t, r, tc.path)
			if w.Code != http.StatusOK {
				t.Fatalf("状态码 = %d, 期望 200", w.Code)
			}
			if got := w.Header().Get("Cache-Control"); got != cacheControlEntry {
				t.Errorf("Cache-Control = %q, 期望 %q", got, cacheControlEntry)
			}
			if string(w.Body.Bytes()) != string(wantIndex) {
				t.Error("响应体与嵌入 index.html 不一致,回退语义回归")
			}
		})
	}

	// assets 产物:任取一个真实存在的 JS 文件,保证请求打到静态文件
	// 命中分支而非回退
	t.Run("assets产物immutable长缓存", func(t *testing.T) {
		entries, err := fs.ReadDir(modelmeter.DistFS, "web/dist/assets")
		if err != nil {
			t.Fatalf("读取嵌入 assets 目录失败: %v", err)
		}
		var asset string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".js") {
				asset = "/assets/" + e.Name()
				break
			}
		}
		if asset == "" {
			t.Skip("构建产物中无 JS 文件可测")
		}

		w := doGet(t, r, asset)
		if w.Code != http.StatusOK {
			t.Fatalf("状态码 = %d, 期望 200", w.Code)
		}
		if got := w.Header().Get("Cache-Control"); got != cacheControlAssets {
			t.Errorf("Cache-Control = %q, 期望 %q", got, cacheControlAssets)
		}
	})
}
