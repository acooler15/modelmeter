// 与后端 error-handling.md 约定的统一 JSON 信封对应的类型定义。
// 所有 api/ 下的接口封装都以此为返回载体。
export interface ApiResponse<T> {
  code: number // 0 成功,非 0 为业务错误码
  message: string // 中文提示
  data: T | null
}
