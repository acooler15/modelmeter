// 流式测试 composable:用 fetch + ReadableStream 手动解析 SSE(EventSource
// 不支持 POST),处理网络分片的 chunk 边界(缓冲到完整事件再解析)、
// 开始推送前的错误信封与 abort 主动中断。
import { ref } from 'vue'

import { ApiError } from '@/api/http'
import { requestTestStream } from '@/api/modelTest'
import type { ApiResponse } from '@/types/api'
import type { TestRecord, TestRequest, TestResult, TestStreamEvent } from '@/types/test'

/** 流式测试正常结束时的交付物。 */
export interface TestStreamOutcome {
  result: TestResult
  record: TestRecord
}

/** 判断异常是否为用户主动中断(abort),调用方据此静默处理。 */
export function isAbortError(e: unknown): boolean {
  return e instanceof DOMException && e.name === 'AbortError'
}

/** 从一帧 SSE 文本中提取 data 内容并解析为事件;空帧/坏帧返回 null。 */
function parseEvent(frame: string): TestStreamEvent | null {
  const data = frame
    .split('\n')
    .filter((line) => line.startsWith('data:'))
    .map((line) => line.slice(5).replace(/^ /, ''))
    .join('\n')
  if (!data) return null
  try {
    const value: unknown = JSON.parse(data)
    if (isStreamEvent(value)) return value
  } catch {
    // 坏帧直接忽略,等待后续事件
  }
  return null
}

/** 运行时收窄:后端只约定 delta/done/error 三种事件形态。 */
function isStreamEvent(value: unknown): value is TestStreamEvent {
  if (typeof value !== 'object' || value === null) return false
  const kind = (value as { type?: unknown }).type
  if (kind === 'delta') return typeof (value as { text?: unknown }).text === 'string'
  if (kind === 'error') return typeof (value as { message?: unknown }).message === 'string'
  if (kind === 'done') {
    const v = value as { result?: unknown; record?: unknown }
    return (
      typeof v.result === 'object' &&
      v.result !== null &&
      typeof v.record === 'object' &&
      v.record !== null
    )
  }
  return false
}

export function useTestStream() {
  const streaming = ref(false)
  // 每次测试新建 AbortController;结束后置空,abort 变为空操作
  let controller: AbortController | null = null

  /** 中断当前流式测试;未在流式中时为空操作。 */
  function abort() {
    controller?.abort()
  }

  /** 发起流式测试:onDelta 在每个内容增量到达时被回调;收到 done 事件时
   * 返回 result/record;error 事件与开始前失败抛出带中文提示的 ApiError;
   * 用户中断抛出 AbortError,由调用方区分处理。 */
  async function start(
    req: TestRequest,
    onDelta: (text: string) => void,
  ): Promise<TestStreamOutcome> {
    streaming.value = true
    controller = new AbortController()
    try {
      let res: Response
      try {
        res = await requestTestStream(req, controller.signal)
      } catch (e) {
        if (isAbortError(e)) throw e
        throw new ApiError(-1, '网络错误:无法连接到后端服务')
      }

      // 开始推送前的失败:响应是统一 JSON 信封而非事件流,按信封语义报错
      const contentType = res.headers.get('Content-Type') ?? ''
      if (!res.ok || !contentType.includes('text/event-stream')) {
        const body = (await res.json().catch(() => null)) as ApiResponse<never> | null
        if (body && body.code !== 0) throw new ApiError(body.code, body.message)
        throw new ApiError(-1, `流式测试请求失败(HTTP ${res.status})`)
      }

      const reader = res.body?.getReader()
      if (!reader) throw new ApiError(-1, '当前浏览器不支持流式读取')

      const decoder = new TextDecoder('utf-8')
      let buffer = ''
      let outcome: TestStreamOutcome | null = null

      // 处理单个事件;返回 true 表示流应结束(done)
      const processEvent = (ev: TestStreamEvent): boolean => {
        if (ev.type === 'delta') {
          onDelta(ev.text)
          return false
        }
        if (ev.type === 'done') {
          outcome = { result: ev.result, record: ev.record }
          return true
        }
        throw new ApiError(-1, ev.message)
      }

      for (;;) {
        const { done, value } = await reader.read()
        if (value) {
          // 分片可能切断事件边界,先缓冲、按空行逐帧解析
          buffer += decoder.decode(value, { stream: true })
          let idx: number
          while ((idx = buffer.indexOf('\n\n')) >= 0) {
            const frame = buffer.slice(0, idx)
            buffer = buffer.slice(idx + 2)
            const ev = parseEvent(frame)
            if (ev && processEvent(ev)) {
              void reader.cancel().catch(() => {}) // 已收到结果,主动释放连接
              if (outcome) return outcome
            }
          }
        }
        if (done) break
      }

      // 流结束但未见 done:容错处理悬挂的最后一帧,否则视为异常中断
      buffer += decoder.decode()
      const last = parseEvent(buffer.trim())
      if (last && processEvent(last) && outcome) return outcome
      throw new ApiError(-1, '流式连接中断,未收到完成事件')
    } finally {
      streaming.value = false
      controller = null
    }
  }

  return { streaming, start, abort }
}
