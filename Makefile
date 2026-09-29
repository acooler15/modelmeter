.PHONY: dev-backend dev-frontend build test lint clean

# 前端开发服务器(vite,/api 已代理到 localhost:8422)
dev-frontend:
	cd web && npm run dev

# 后端开发运行(需先构建过前端,否则页面为占位提示)
dev-backend:
	go run ./cmd/server

# 完整构建:先产出前端静态文件,再编译单二进制(Windows 下生成 .exe)
build:
	cd web && npm run build
	go build -o bin/modelmeter.exe ./cmd/server

# 后端测试 + 前端 lint + 类型检查构建
test:
	go test ./internal/... ./cmd/...
	go vet ./internal/... ./cmd/...
	cd web && npm run lint
	cd web && npm run build

clean:
	rm -rf bin web/dist
	mkdir web/dist
	touch web/dist/.gitkeep
