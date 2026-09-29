// New API 模型费率接口封装:配置查询/保存、费率拉取与成本估算。
import { httpGet, httpPost, httpPut } from './http'

import type {
  EstimateResult,
  NewAPIConfigInput,
  NewAPIConfigView,
  RateEntry,
} from '@/types/newapi'

/** 获取 New API 配置视图;令牌只回脱敏值。 */
export function getNewAPIConfig(): Promise<NewAPIConfigView> {
  return httpGet<NewAPIConfigView>('/api/newapi/config')
}

/** 保存 New API 配置;input.token 留空表示沿用原令牌。 */
export function saveNewAPIConfig(input: NewAPIConfigInput): Promise<NewAPIConfigView> {
  return httpPut<NewAPIConfigView>('/api/newapi/config', input)
}

/** 实时拉取模型费率表,后端按模型名排序返回。 */
export function fetchNewAPIRates(): Promise<RateEntry[]> {
  return httpGet<RateEntry[]>('/api/newapi/rates')
}

/** 按费率表估算单次调用成本;未配置/未命中/失败时 available=false。 */
export function estimateCost(
  model: string,
  promptTokens: number,
  completionTokens: number,
): Promise<EstimateResult> {
  return httpPost<EstimateResult>('/api/newapi/estimate', {
    model,
    prompt_tokens: promptTokens,
    completion_tokens: completionTokens,
  })
}
