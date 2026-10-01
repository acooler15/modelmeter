// 窄屏判定 composable:是否处于 ≤768px 的移动端视口。
// 模块级单例:整个应用共享同一个 matchMedia 监听,避免每个组件重复挂监听器;
// 视口状态属于设备能力而非业务状态,因此不进 Pinia。
import { ref, type Ref } from 'vue'

// 断点与全局样式的 @media (max-width: 768px) 保持一致,两侧改动须同步
const MOBILE_QUERY = '(max-width: 768px)'

const mql = window.matchMedia(MOBILE_QUERY)
const isMobile = ref(mql.matches)

// 视口跨越断点时同步状态;无 SSR 场景,模块顶层挂监听安全
mql.addEventListener('change', (e) => {
  isMobile.value = e.matches
})

/** 是否窄屏视口(≤768px)。返回只读 ref,消费方不得写入,状态仅由媒体查询驱动。 */
export function useIsMobile(): Readonly<Ref<boolean>> {
  return isMobile
}
