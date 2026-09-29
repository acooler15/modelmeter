// 健康检查接口封装。
import { httpGet } from './http'

/** 服务健康状态。 */
export interface HealthStatus {
  status: string
}

/** 探测后端服务与数据库连通性。 */
export function getHealth(): Promise<HealthStatus> {
  return httpGet<HealthStatus>('/api/health')
}
