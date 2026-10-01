# 移动端适配规范

> 窄屏(≤768px)适配约定。源自 10-02-frontend-mobile-responsive 任务,
> 新增页面或组件时按本规范保持移动端可用。

---

## 概述

- **单一断点 768px**:≤768px 视为移动端,不做多断点平板精调。
  断点定义在两处,必须同步修改:`web/src/composables/useIsMobile.ts`
  (JS 侧 matchMedia)与 `web/src/styles/index.css`(CSS 侧媒体查询)。
- 三类手段各司其职:
  1. 跨页面共性模式 → `styles/index.css` 的 `@media (max-width: 768px)` 全局块;
  2. 组件私有固定宽度 → 各组件 `<style scoped>` 内的媒体查询;
  3. Element Plus 组件属性切换 → `useIsMobile()` 响应式绑定。

---

## 视口判断:useIsMobile

```ts
import { useIsMobile } from '@/composables/useIsMobile'

const isMobile = useIsMobile() // Readonly<Ref<boolean>>,消费方只读
```

- 模块级单例(matchMedia 只挂一次监听),是"composable 模块顶层不建共享
  ref"约定的**唯一例外**:视口是设备能力而非业务状态,不需要 Pinia。
- 仅用于 EP 组件属性切换与布局分支(`v-if`),不要用它复制两套大模板。

---

## 全局类名约定(新页面必须沿用)

`styles/index.css` 的移动端块按类名生效,新页面的卡片头与工具栏**必须**
沿用以下类名,否则窄屏不换行:

- `.card-header` — el-card 头部 `display:flex; justify-content:space-between` 行
- `.toolbar` — 页面内的筛选/操作工具栏 flex 行

全局移动端块现有内容(修改时保持范围最小):`.el-main`/`.el-card__body`
内边距收窄、上述两类名换行、`.el-message-box` 宽度保护。

---

## Element Plus 响应式绑定模式

```vue
<!-- 弹窗:窄屏全屏,桌面保持定宽 -->
<el-dialog v-model="visible" title="…" width="480px" :fullscreen="isMobile">
  <!-- 表单:窄屏标签置顶(label-width 自动失效),桌面右置 -->
  <el-form :label-position="isMobile ? 'top' : 'right'" label-width="90px">
```

```vue
<!-- 描述列表:窄屏降为 1 列 -->
<el-descriptions :column="isMobile ? 1 : 3" border>
```

- 所有此类绑定必须经过 `isMobile`,禁止用 CSS hack 改 EP 属性。

---

## 固定宽度与表格

- 组件私有的固定 px 宽度(下拉、输入框等)必须在**本组件 scoped 样式**
  里加媒体查询收窄为 `width: 100%`,不要写进全局 CSS:

```css
@media (max-width: 768px) {
  .provider-select { width: 100%; }
}
```

- `el-table` 移动端**保持组件内横向滚动**,不改造为卡片列表;操作列用
  `fixed="right"` 保证滚动时可见。页面级(body)不允许出现横向滚动条。

---

## 常见错误:全局覆盖被懒加载样式反超

**症状**:入口 CSS(如 `styles/index.css`)里对 `.el-main`、`.el-card__body`
这类 EP 组件的同特异性覆盖规则,dev 与 build 下都不生效。

**原因**:路由全部懒加载,EP 组件样式在异步 chunk 的 CSS 里,运行时以
`<link>` 追加到 head 末尾,**晚于入口 CSS**;同特异性时后来者胜。
实例:`.el-main { padding: 12px }` 被懒加载的
`.el-main { padding: var(--el-main-padding) }` 反超。

**正确**:提升特异性使顺序无关;媒体查询不改变级联权重,救不了它:

```css
/* 错:同特异性,被懒加载 chunk 反超 */
@media (max-width: 768px) { .el-main { padding: 12px } }

/* 对:提高特异性 */
@media (max-width: 768px) { body .el-main { padding: 12px } }
```

> 注意 EP 把 `--el-main-padding: 20px` 定义在 `.el-main` 元素自身上,
> 用 `:root { --el-main-padding: 12px }` 覆盖无效(元素级定义赢过继承)。
