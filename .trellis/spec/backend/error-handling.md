# 错误处理

> 本项目的错误处理方式。

---

## 概述

Go 层用 `error` 值逐层传递;HTTP 层通过统一 JSON 信封返回。业务错误集中
定义在 `internal/apperr` 包,携带业务错误码和面向用户的中文提示。

---

## 错误类型

```go
package apperr

// Error 业务错误:业务码 + 用户可读的中文消息 + 可选的底层错误。
type Error struct {
    Code    int    // 业务错误码,非 0
    Message string // 返回给前端的中文提示
    Err     error  // 底层错误,可为 nil
}

func (e *Error) Error() string { ... }

// 常用构造函数(错误码在 errors.go 中集中定义为常量):
func New(code int, message string) *Error
func Wrap(code int, message string, err error) *Error
```

- 错误码常量集中放在 `internal/apperr/codes.go`,按资源分段(如
  meter 类 1xxx)。
- service 层返回 `*apperr.Error`;底层库错误(如 GORM)用 `Wrap` 包一层,
  不要裸传。

---

## 处理模式

- service / model 层:错误只返回不打印,由最外层统一记录。
- handler 层统一走一个辅助函数:

```go
// 输出统一信封;apperr 用其 Code/Message,其余一律按 500 处理
response.Fail(c, err)
```

- handler 内禁止 `panic`;不可恢复错误交给 recover 中间件兜底并记日志。
- 判断错误用 `errors.As` / `errors.Is`,不要对 `Error()` 字符串做比较。

---

## API 错误响应

统一信封格式:

```json
{ "code": 0, "message": "ok", "data": { } }
```

- 成功:`code` 为 0,`message` 为 `"ok"`,业务数据放 `data`。
- 失败:`code` 为业务错误码,`message` 为**中文**可读提示,`data` 为 null。
- HTTP 状态码:参数/业务错误 400,未找到 404,服务器内部错误 500;
  前端主要依据 `code` 判断。
- 500 的 `message` 固定为通用文案(如"服务器内部错误"),细节只进日志,
  不暴露给客户端。

---

## 常见错误

- 用 `fmt.Errorf` 造临时错误返回给前端 —— 用户看到的是英文技术信息;
  应改用 `apperr`。
- 重复包装导致错误码丢失;`Wrap` 只包一次。
- 吞错:`_ = doSomething()`。要么处理,要么显式注释为什么可以忽略。
