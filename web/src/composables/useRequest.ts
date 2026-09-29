// 统一的数据请求 composable:管理 loading / error / data,
// 失败时统一弹中文错误提示,组件只处理成功分支。
import { ref, type Ref } from 'vue'

import { ApiError } from '@/api/http'

export function useRequest<T, Args extends unknown[]>(fn: (...args: Args) => Promise<T>) {
  const data: Ref<T | undefined> = ref(undefined)
  const loading = ref(false)
  const error: Ref<ApiError | null> = ref(null)

  /** 执行请求;成功返回业务数据,失败弹提示并返回 undefined。 */
  async function run(...args: Args): Promise<T | undefined> {
    loading.value = true
    error.value = null
    try {
      const result = await fn(...args)
      data.value = result
      return result
    } catch (e) {
      error.value =
        e instanceof ApiError ? e : new ApiError(-1, '请求失败,请稍后重试')
      ElMessage.error(error.value.message)
      return undefined
    } finally {
      loading.value = false
    }
  }

  return { data, loading, error, run }
}
