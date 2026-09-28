# 质量规范

> 前端代码质量标准。

---

## 概述

- 语言:TypeScript(strict),见 [type-safety.md](./type-safety.md)。
- Lint:ESLint(flat config,含 `eslint-plugin-vue` + typescript-eslint)+
  Prettier 格式化;提交前 `npm run lint` 必须通过。
- 类型检查:`vue-tsc --noEmit`;构建命令统一为
  `npm run build`(内部先跑 vue-tsc 再 vite build)。
- **注释一律用中文**,解释"为什么";标识符用英文。

---

## 禁止模式

- `any`、`@ts-ignore`(见 type-safety.md)。
- 组件内直接 `fetch` / `axios` 而不走 `src/api/` 层。
- 直接操作 DOM(`document.querySelector`);应使用 ref 与响应式数据。
- 未处理的 Promise rejection:异步调用要么 `await` + 错误处理,要么交给
  `useRequest` 统一处理。
- 在 `<style>` 里写全局选择器污染其他页面(必须 `scoped`)。
- Options API、混用两种组件风格。

---

## 必须模式

- 新页面走标准纵向切片:`api/` 函数 → `views/` 页面 → 路由注册。
- 用户可见的文案(按钮、提示、校验消息)全部使用**中文**。
- 错误提示统一 `ElMessage.error`(由 `useRequest` 触发),不在组件里
  各写一套。
- 新依赖需证明现有依赖无法实现,并在任务记录中说明理由;保持依赖面小。

---

## 测试要求

当前为小项目,前端不强制单元测试;约定如下:

- 核心组合式函数(如 `useRequest`)建议用 Vitest 覆盖,放
  `src/composables/__tests__/`。
- 测试文件命名 `*.test.ts`,用例描述用中文。
- 不为纯展示组件写快照测试。

---

## 代码评审清单

- [ ] 是否通过 `npm run lint` 与 `npm run build`(含 vue-tsc)?
- [ ] 请求是否都走 `api/` 层、类型是否与后端信封一致?
- [ ] 页面文案是否为中文、校验提示是否友好?
- [ ] 组件是否符合 `<script setup>` + scoped 样式约定?
- [ ] 新增依赖是否有必要?
