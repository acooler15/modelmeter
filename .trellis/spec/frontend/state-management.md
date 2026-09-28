# 状态管理

> 本项目的状态管理约定(Pinia)。

---

## 概述

- 状态库:**Pinia**,通过 `createPinia` 在 `main.ts` 装配。
- store 放 `src/stores/`,一个领域一个文件(`user.ts`、`meter.ts`)。
- 大多数页面数据不需要进 store:服务端数据由页面内的 `useRequest` 持有,
  只有跨页面共享的才提升为 store。

---

## 状态分类

| 类别 | 放哪里 | 示例 |
|------|--------|------|
| 组件私有 | 组件内 `ref` | 弹窗开关、表单草稿 |
| 跨组件/页面共享 | Pinia store | 当前登录用户、全局配置 |
| 服务端数据(页面私有) | `useRequest` composable | 列表页数据 |
| 服务端数据(全局共享) | Pinia store + 显式刷新动作 | 用户信息 |
| URL 状态 | `route.query` / `router.replace` | 列表页的页码、筛选条件 |

列表页的页码与筛选条件应同步到 URL query,刷新与分享后能还原。

---

## Store 编写模式

统一用 setup store 写法(组合式),与 `<script setup>` 风格一致:

```ts
// src/stores/user.ts
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { fetchCurrentUser } from '@/api/user'

/** 当前登录用户信息。 */
export const useUserStore = defineStore('user', () => {
  const user = ref<UserInfo | null>(null)

  /** 拉取并缓存当前用户,登录后调用。 */
  async function load() {
    user.value = await fetchCurrentUser()
  }

  const isLoggedIn = computed(() => user.value !== null)

  return { user, isLoggedIn, load }
})
```

- `defineStore` 第一个参数用领域名词小写(`'user'`)。
- 异步动作(请求后端)直接写成 `async function` 并导出。
- 组件里调用 store 动作后由动作内部更新 state,组件不改 store 的字段
  ( `$patch` 也不在组件里用)。

---

## 何时提升为全局状态

同时满足以下条件才建 store:两个以上不相干的页面/组件需要;数据有明确
的单一来源;更新路径清晰(由动作更新)。不确定时先留在页面内,出现第二
个使用方再提升。

---

## 常见错误

- 把列表页数据整体塞进 store,离开页面也不清理,导致下次进入显示旧数据。
- 组件里直接改 store 字段绕过动作,状态变化无从追踪。
- 在 store 外用 `storeToRefs` 之外的解构方式丢失响应性(解构 store 必须
  用 `storeToRefs`)。
