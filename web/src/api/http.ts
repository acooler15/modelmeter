// 后端 HTTP 请求统一封装:解析统一信封,业务失败抛 ApiError,
// 由 useRequest 统一转成中文提示;组件不得直接使用 fetch。
import type { ApiResponse } from '@/types/api'

/** 后端业务错误:携带业务错误码与面向用户的中文提示。 */
export class ApiError extends Error {
  readonly code: number

  constructor(code: number, message: string) {
    super(message)
    this.code = code
  }
}

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  let res: Response
  try {
    res = await fetch(url, {
      headers: { 'Content-Type': 'application/json' },
      ...init,
    })
  } catch {
    // fetch 仅在网络层失败时 reject,与业务错误区分提示
    throw new ApiError(-1, '网络错误:无法连接到后端服务')
  }

  const body = (await res.json().catch(() => null)) as ApiResponse<T> | null
  if (!body) {
    throw new ApiError(-1, '响应格式错误')
  }
  if (body.code !== 0) {
    throw new ApiError(body.code, body.message)
  }
  if (body.data === null) {
    throw new ApiError(-1, '响应缺少数据')
  }
  return body.data
}

/** 发起 GET 请求并返回信封中的业务数据。 */
export function httpGet<T>(url: string): Promise<T> {
  return request<T>(url)
}

/** 发起 POST 请求并返回信封中的业务数据。 */
export function httpPost<T>(url: string, body?: unknown): Promise<T> {
  return request<T>(url, {
    method: 'POST',
    body: body === undefined ? undefined : JSON.stringify(body),
  })
}

/** 发起 PUT 请求并返回信封中的业务数据。 */
export function httpPut<T>(url: string, body?: unknown): Promise<T> {
  return request<T>(url, {
    method: 'PUT',
    body: body === undefined ? undefined : JSON.stringify(body),
  })
}

/** 发起 DELETE 请求并返回信封中的业务数据。 */
export function httpDelete<T>(url: string): Promise<T> {
  return request<T>(url, { method: 'DELETE' })
}
