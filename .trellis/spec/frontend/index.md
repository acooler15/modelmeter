# 前端开发规范

> ModelMeter 前端(Vue 3)开发约定。

---

## 概述

技术栈基线:**Vue 3(`<script setup>` + TypeScript)+ Vite +
Element Plus(按需自动导入)+ vue-router + Pinia**。构建产物输出到
`web/dist`,由 Go 后端嵌入单二进制,与后端通过 `/api` 下的统一 JSON 信封
通信。

> 语言约定:所有规范文档使用中文;代码注释使用中文;标识符用英文,
> 页面可见文案用中文。

---

## 规范索引

| 文档 | 内容 | 状态 |
|------|------|------|
| [目录结构](./directory-structure.md) | src 布局、api/views/composables 分工、命名 | 已填写 |
| [组件规范](./component-guidelines.md) | `<script setup>` 结构、props/emits、样式、可访问性 | 已填写 |
| [组合式函数规范](./composable-guidelines.md) | composables 编写模式、useRequest 数据请求 | 已填写 |
| [状态管理](./state-management.md) | Pinia setup store、状态分类与提升标准 | 已填写 |
| [类型安全](./type-safety.md) | strict TS、API 信封类型、禁止 any | 已填写 |
| [质量规范](./quality-guidelines.md) | lint/类型检查、禁止/必须模式、评审清单 | 已填写 |
| [移动端适配](./mobile-adaptation.md) | 768px 断点、useIsMobile、响应式绑定与懒加载 CSS 陷阱 | 已填写 |

---

## 维护说明

规范记录的是项目实际约定。代码演进导致约定变化时,同步更新对应文件,
保持规范与现实一致。
