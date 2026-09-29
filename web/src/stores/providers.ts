// 接口配置 store:providers 列表被模型列表页(下拉选择)与接口配置页共用,
// 按 state-management 规范提升为项目首个 Pinia store,单一数据源由 load 动作更新。
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { ApiError } from '@/api/http'
import { listProviders } from '@/api/provider'
import type { ProviderView } from '@/types/provider'

/** 接口配置列表 store:跨页面共享,组件只调用 load 动作,不直接改字段。 */
export const useProvidersStore = defineStore('providers', () => {
  const list = ref<ProviderView[]>([])
  const loading = ref(false)
  /** 是否完成过一次成功加载;已加载时默认跳过重复请求,变更后用 force 刷新。 */
  const loaded = ref(false)

  const hasProviders = computed(() => list.value.length > 0)

  /** 拉取接口配置列表;失败时统一弹后端中文提示,不向调用方抛错,
   * 因此调用方可以安全地 `void store.load()` 而不产生未处理的 Promise rejection。 */
  async function load(force = false) {
    if (loading.value) return
    if (loaded.value && !force) return
    loading.value = true
    try {
      list.value = await listProviders()
      loaded.value = true
    } catch (e) {
      ElMessage.error(e instanceof ApiError ? e.message : '请求失败,请稍后重试')
    } finally {
      loading.value = false
    }
  }

  return { list, loading, loaded, hasProviders, load }
})
