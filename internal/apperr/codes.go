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

// 上游访问类错误码:1xxx 段,模型列表/模型测试/费率查询共用。
// 网络错误与鉴权错误必须可区分(P0 硬性要求),细分与文案见各任务 design.md。
const (
	CodeUpstreamNetwork     = 1500 // 无法连接上游服务(连接失败/超时/DNS/TLS)
	CodeUpstreamAuth        = 1501 // 上游鉴权失败(HTTP 401/403)
	CodeUpstreamBadResponse = 1502 // 上游响应异常(非 2xx 状态码或解析失败)
)

// 模型测试类错误码:2xxx 段。上游网络/鉴权错误沿用 1500/1501,
// 使模型测试与模型列表对同类故障的提示保持一致。
const (
	CodeTestInvalid           = 2401 // 测试参数缺失或非法(协议不支持、模型/用户消息为空等)
	CodeUpstreamRequestFailed = 2500 // 上游请求失败(非 401/403 的非 2xx 状态码)
	CodeUpstreamParseFailed   = 2502 // 上游响应解析失败(响应体不符合所选协议结构)
)

// New API 费率类错误码:3xxx 段,细分与文案见任务 design.md。
const (
	CodeNewAPINotConfigured = 3401 // New API 配置缺失或不完整(地址/令牌未配置)
	CodeNewAPINetwork       = 3500 // 无法连接 New API 服务(连接失败/超时/DNS/TLS)
	CodeNewAPIAuth          = 3501 // New API 鉴权失败(HTTP 401/403)
	CodeNewAPIBadResponse   = 3502 // New API 响应异常(非 2xx 状态码或解析失败)
)

// Agent 配置类错误码:4xxx 段,细分与文案见任务 design.md。
const (
	CodeAgentUnknown     = 4401 // 不支持的 Agent 名称
	CodeAgentFileIO      = 4402 // 配置文件读取/写回/备份失败,或内容不是合法 JSON
	CodeAgentInvalid     = 4403 // 提交的配置值非法(必填缺失、不在可选项内等)
	CodeAgentNotFound    = 4404 // 配置文件不存在(含无备份可还原)
	CodeAgentUnsupported = 4405 // 该 Agent 不支持所请求的操作(能力接口未实现)
)
