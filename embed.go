// Package modelmeter 仅承载前端构建产物的嵌入声明。
//
// Go 的 //go:embed 无法引用上级目录,而前端产物按规范固定输出到仓库根的
// web/dist,因此嵌入声明必须放在与 web/dist 同级的模块根,由 internal/web
// 引用并托管为 SPA。
package modelmeter

import "embed"

// DistFS 前端构建产物(web/dist),由 internal/web 托管为 SPA 页面。
//
//go:embed all:web/dist
var DistFS embed.FS
