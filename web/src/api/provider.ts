// 接口配置接口封装:列表、新增、编辑、删除。
import { httpDelete, httpGet, httpPost, httpPut } from './http'

import type { ProviderInput, ProviderView } from '@/types/provider'

/** 获取全部接口配置,按创建时间正序返回。 */
export function listProviders(): Promise<ProviderView[]> {
  return httpGet<ProviderView[]>('/api/providers')
}

/** 新增接口配置,名称、Base URL、API Key 均必填。 */
export function createProvider(input: ProviderInput): Promise<ProviderView> {
  return httpPost<ProviderView>('/api/providers', input)
}

/** 编辑接口配置;input.api_key 为空表示沿用原 Key。 */
export function updateProvider(id: number, input: ProviderInput): Promise<ProviderView> {
  return httpPut<ProviderView>(`/api/providers/${id}`, input)
}

/** 删除接口配置,返回被删 ID。 */
export function deleteProvider(id: number): Promise<{ id: number }> {
  return httpDelete<{ id: number }>(`/api/providers/${id}`)
}
