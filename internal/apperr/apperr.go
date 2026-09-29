// Package apperr 定义带业务错误码的领域错误,消息面向用户、一律使用中文。
package apperr

// Error 业务错误:业务错误码 + 用户可读的中文提示 + 可选的底层错误。
type Error struct {
	Code    int    // 业务错误码,非 0,取值见 codes.go
	Message string // 返回给前端的中文提示
	Err     error  // 底层错误,可为 nil,只进日志不返回给客户端
}

// Error 实现 error 接口;包含底层错误时一并输出,便于日志排查。
func (e *Error) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

// Unwrap 暴露底层错误,使 errors.Is / errors.As 可以穿透到根因。
func (e *Error) Unwrap() error { return e.Err }

// New 创建不含底层错误的业务错误。
func New(code int, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Wrap 在底层错误外包一层业务错误,保留错误链;只包一次,不要重复包装。
func Wrap(code int, message string, err error) *Error {
	return &Error{Code: code, Message: message, Err: err}
}
