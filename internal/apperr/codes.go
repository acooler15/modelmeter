package apperr

// 业务错误码集中定义。
//
// 分段约定:通用段与 HTTP 状态码对齐(400/404/500),便于记忆与映射;
// 资源类错误码从 1000 起按资源分段(如接口配置 1xxx、模型测试 2xxx),
// 新增资源时在此登记。
const (
	CodeBadRequest = 400 // 参数或业务校验错误
	CodeNotFound   = 404 // 资源不存在
	CodeInternal   = 500 // 服务器内部错误
)

// 接口配置(provider)类错误码:1xxx 段,细分与文案见任务 design.md。
const (
	CodeProviderInvalid   = 1401 // 参数缺失、ID 非法或 Base URL 不是合法 http(s) 地址
	CodeProviderDuplicate = 1402 // 名称已存在
	CodeProviderNotFound  = 1404 // 接口配置不存在
)
