// 模型列表接口封装:经后端代理拉取上游 /v1/models,浏览器不直连上游。
import { httpGet } from './http'

import type { ModelInfo } from '@/types/model'

/** 拉取指定接口配置的模型列表;providerId 为后端保存的配置 ID。 */
export function fetchModels(providerId: number): Promise<ModelInfo[]> {
  return httpGet<ModelInfo[]>(`/api/providers/${providerId}/models`)
}
