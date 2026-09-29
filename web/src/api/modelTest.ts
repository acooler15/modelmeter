// 模型测试接口封装:非流式与记录走统一信封封装;流式端点需要手动解析
// SSE 帧,单独提供返回原始 Response 的函数,URL 仍收敛在本层。
import { httpDelete, httpGet, httpPost } from './http'

import type { TestRecord, TestRequest, TestResult } from '@/types/test'

/** 非流式测试的信封 data:结果与本次落库的记录。 */
export interface TestRunData {
  result: TestResult
  record: TestRecord
}

/** 发起非流式测试,返回结果与本次记录。 */
export function runTest(req: TestRequest): Promise<TestRunData> {
  return httpPost<TestRunData>('/api/test', req)
}

/** 拉取最近测试记录,创建时间倒序;limit 上限 200。 */
export function fetchRecords(limit = 100): Promise<TestRecord[]> {
  return httpGet<TestRecord[]>(`/api/test/records?limit=${limit}`)
}

/** 清空全部测试记录,返回删除条数。 */
export function clearRecords(): Promise<{ deleted: number }> {
  return httpDelete<{ deleted: number }>('/api/test/records')
}

/** 发起流式测试请求,返回原始 Response 供 useTestStream 手动解析 SSE。
 * 唯一绕过统一信封封装的接口:开始推送前的失败需按信封语义解读。 */
export async function requestTestStream(req: TestRequest, signal: AbortSignal): Promise<Response> {
  return fetch('/api/test/stream', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
    signal,
  })
}
