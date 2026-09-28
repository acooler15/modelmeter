# 组件规范

> 本项目 Vue 组件的编写方式。

---

## 概述

统一使用 Vue 3 组合式 API 的 `<script setup lang="ts">` 写法,不使用
Options API。UI 组件用 Element Plus,通过 `unplugin-vue-components` +
`unplugin-auto-import` 按需自动导入,不手动 import 组件与样式。

---

## 组件结构

单文件组件的区块顺序固定:`<script setup>` → `<template>` → `<style>`。

```vue
<script setup lang="ts">
// 中文注释说明组件用途与关键决策
import { ref, computed } from 'vue'
import type { Meter } from '@/api/meter'

// props 与 emits 必须用类型化声明,且放在 script 最前面
const props = defineProps<{ meter: Meter; removable?: boolean }>()
const emit = defineEmits<{ remove: [id: number] }>()

const label = computed(() => `${props.meter.name}(#${props.meter.id})`)
</script>

<template>
  <el-card>{{ label }}</el-card>
</template>

<style scoped>
/* 样式默认 scoped;需要覆盖 Element Plus 内部时用 :deep() */
</style>
```

---

## Props 与 Emits

- 只用类型声明 `defineProps<{...}>()`,不用运行时对象声明;需要默认值时
  用 `withDefaults(defineProps<{...}>(), { ... })`。
- props 不在组件内修改;需要变更语义时 emit 事件由父组件处理。
- emits 用带参元组类型 `defineEmits<{ change: [val: string] }>()`。
- 不用 `defineExpose` 暴露内部状态,除非是明确的复用组件(如表单)。

---

## 样式模式

- 默认 `<style scoped>`;全局样式只放 `src/styles/`。
- 覆盖 Element Plus 内部类用 `:deep(...)`,禁止全局裸改组件类名。
- 主题色等设计变量集中写在 `styles/variables.scss`,通过 Element Plus 的
  CSS 变量机制覆盖,不在组件里写死颜色。
- 不引入 CSS-in-JS、Tailwind;普通 SCSS 即可。

---

## 可访问性

- Element Plus 组件自带的可访问性不要破坏:按钮用 `el-button`,不要用
  `div` + click 代替按钮。
- 表单控件必须有 label(`el-form-item` 的 `label` 属性)。
- 图标按钮提供 `aria-label` 或中文 `title`。

---

## 常见错误

- 在 `views/` 组件里直接写 `fetch` 而不走 `api/` 层。
- 用 Options API 或混用两种 API 风格。
- 手动 `import { ElButton } from 'element-plus'`(自动导入已覆盖)。
- 忘写 `scoped` 导致样式互相污染。
