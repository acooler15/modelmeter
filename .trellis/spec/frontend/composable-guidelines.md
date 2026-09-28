# 组合式函数(Composables)规范

> 本项目可复用状态逻辑的封装方式。Vue 中没有 React 的 "hooks",
> 对应概念是 composables,文件与函数均以 `use` 开头。

---

## 概述

组合式函数放 `src/composables/`,一个文件一个函数,文件名与函数同名
(`useRequest.ts` → `useRequest()`)。用于封装"多个组件都要用的有状态
逻辑"(请求、轮询、分页、表单校验等),纯函数工具一律放 `utils/`,
不要写成 composable。

---

## 编写模式

```ts
// src/composables/usePagination.ts
import { ref, computed } from 'vue'

/** 分页逻辑复用:接收总数,返回页码状态与翻页方法。 */
export function usePagination(total: Ref<number>, pageSize = 20) {
  const page = ref(1)
  const pageCount = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

  function nextPage() { if (page.value < pageCount.value) page.value++ }

  return { page, pageCount, nextPage }
}
```

- 输入用 `ref`/`getter`,返回值统一是包含 `ref` 与函数的普通对象;
  调用方自行解构。
- 不在 composable 里直接访问 Pinia store 或 router 全局实例,需要时通过
  参数传入,保持可测试。
- 事件监听、定时器必须在 `onScopeDispose` 中清理,避免内存泄漏。

---

## 数据请求

小项目约定:**手写一个 `useRequest` composable 统一管理 loading /
error / data**,不引入 vue-query / SWR 这类库。

```ts
// 封装 api/ 中的函数,返回响应式的 data/loading/error
const { data: meters, loading, error, run } = useRequest(listMeters)
```

- 请求函数一律来自 `src/api/`,composable 不直接拼 URL。
- 组件挂载时的首次请求放在页面组件里调用,composable 保持无副作用。
- 错误提示统一由 `useRequest` 内部弹 Element Plus 的 `ElMessage.error`,
  组件只处理成功分支;`error` 供需要特殊处理的组件使用。

---

## 命名约定

- 函数与文件都以 `use` 开头,驼峰式:`useRequest`、`usePagination`。
- 返回的 ref 命名为名词(`loading`、`data`),动作为动词(`run`、`nextPage`)。

---

## 常见错误

- 把无状态的纯函数写成 composable(应放 `utils/`)。
- composable 内部 `setTimeout`/`addEventListener` 后不清理。
- 在 composable 模块顶层创建共享 ref 导致所有组件共用一份状态 ——
  共享状态应放 Pinia,composable 只在函数体内创建状态。
