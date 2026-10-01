# 技术设计:前端移动端适配

## 总体策略

单一断点(768px,≤768 为移动端)+「CSS 优先、JS 最少」:

1. **全局样式**(`web/src/styles/index.css`):处理跨页面的共性模式——卡片头/工具栏换行、容器内边距收窄、按钮组换行、MessageBox 宽度保护。
2. **组件内 scoped 媒体查询**:处理组件私有的固定 px 宽度(各页下拉/输入框),放在各自 `<style scoped>` 内,不污染全局。
3. **`isMobile` 响应式绑定**:处理 CSS 做不到的 EP 组件属性——弹窗 `fullscreen`、表单 `label-position`、`el-descriptions :column`、MainLayout 布局分支。

不新增依赖;`@element-plus/icons-vue` 未安装,汉堡按钮用纯 CSS(三条横线 span)实现。

## 移动端判定:`useIsMobile` composable

新增 `web/src/composables/useIsMobile.ts`,模块级单例(避免每个组件重复挂 MediaQueryListener):

```ts
// 是否窄屏视口(≤768px)。模块级单例:整个应用共享一个 matchMedia 监听。
const mql = window.matchMedia('(max-width: 768px)')
const isMobile = ref(mql.matches)
mql.addEventListener('change', (e) => { isMobile.value = e.matches })

export function useIsMobile(): Readonly<Ref<boolean>> {
  return isMobile
}
```

无 SSR 场景,模块顶层 `matchMedia` 安全。返回只读 ref,消费方不得写入。

## MainLayout 改造(`web/src/layouts/MainLayout.vue`)

现状:固定 `el-aside width="220px"`(第 10 行)+ 顶栏纯文案,窄屏下侧边栏挤占约 2/3 视口。

方案:

- 菜单项收敛为 `menuItems` 常量数组(`{ index, label }`),桌面侧边栏与移动抽屉共用 `v-for`,消除双份维护。
- **桌面分支**(`!isMobile`):结构与现状一致——`el-aside`(brand + el-menu router)+ header 文案。
- **移动分支**(`isMobile`):
  - 隐藏 aside;header 左侧为 CSS 汉堡按钮(3 条横线,点击切换 `menuOpen`)+ 品牌 "ModelMeter"。
  - `el-drawer v-model="menuOpen" direction="ltr" size="240px" :with-header="false"`,内部复用同一 `menuItems` 渲染 `el-menu router`。
  - `watch(() => route.path, () => { menuOpen.value = false })`:点击任意菜单跳转后抽屉自动收起。
- 桌面端 DOM 不渲染抽屉(`v-if="isMobile"`),保持回归零变更。

## 全局样式(`web/src/styles/index.css`,全部包在 `@media (max-width: 768px)`)

| 规则 | 目的 | 证据 |
|------|------|------|
| `.el-main { padding: 12px }` | 主区默认 20px 内边距收窄 | index.css 现无 main 相关规则 |
| `.el-card__body { padding: 12px }` | 卡片内边距收窄 | EP 默认 20px |
| `.card-header, .toolbar { flex-wrap: wrap; row-gap: 8px }` | 8 处卡片头/工具栏换行 | 各视图 scoped `display:flex; justify-content:space-between` 均无 wrap |
| `.el-radio-group { flex-wrap: wrap }` | 协议按钮组换行 | ModelTestView 271-277 行三个长名 radio-button |
| `.el-message-box { width: calc(100vw - 24px) !important }` | MessageBox 默认 420px,375px 屏会溢出 | 6 处 ElMessageBox.confirm |

说明:`.card-header/.toolbar` 是 scoped 类名,但全局规则可直接作用于元素(scoped 只约束其自身规则);scoped 规则均未设置 `flex-wrap`,无优先级冲突。

## 各视图改动清单

### 1. ProviderList.vue(接口配置)
- 5 列表格(min≈890px)、`el-dialog width="480px"`(261-265 行)、`label-width="90px"`(266 行)。
- 改动:script 引入 `useIsMobile`;dialog 加 `:fullscreen="isMobile"`;form 加 `:label-position="isMobile ? 'top' : 'right'"`。表格依赖全局滚动即可。

### 2. ModelListView.vue(模型列表)
- `.toolbar` 无 wrap(176-180 行);`.provider-select`/`.keyword-input` 各 240px(182-189 行)。
- 改动:scoped 媒体查询内 `.provider-select, .keyword-input { width: 100% }`,并取消 `.keyword-input` 的 `margin-left: auto`(移到媒体查询内置 0)。控件逐行堆叠,按钮行随 flex-wrap 排布。

### 3. ModelTestView.vue(模型测试)
- `label-width="130px"`(240 行)、`.provider-select` 260px(496-498 行)、协议 radio-group、结果 `el-descriptions :column="3"`(345 行)、记录表 7 列(389-417 行)、详情 `el-dialog width="680px"` + 内部 `:column="2"`(421-422 行)。
- 改动:
  - 发起测试 form 与详情弹窗 form:`label-position` 响应式。
  - scoped 媒体查询:`.provider-select { width: 100% }`。
  - 结果 descriptions:`:column="isMobile ? 1 : 3"`;详情弹窗:`:fullscreen="isMobile"`,内部 descriptions `:column="isMobile ? 1 : 2"`。
  - 记录表保持组件内横向滚动(7 列不做裁剪,非目标)。

### 4. RateView.vue(模型费率)
- 头部 `.configured-hint` 为完整 base_url+打码令牌、头部无 wrap(104-110 行);`label-width="110px"`;`.filter-input` 220px(240-242 行);费率表 5 列(181-207 行)。
- 改动:form `label-position` 响应式;scoped 媒体查询 `.filter-input { width: 100% }`;`.configured-hint` 加 `word-break: break-all`(对桌面无副作用);头部换行由全局规则覆盖。

### 5. AgentView.vue(Agent 配置容器)
- 仅 card-header 无 wrap,由全局 wrap 规则覆盖;`el-tabs` 头自带横向滚动,无需改动。

### 6. AgentModelCard.vue(Agent 页核心组件,风险最集中)
- 头部最多 4 个全尺寸按钮并排(562-582 行,`.card-actions { gap: 0 }` 无 wrap);`.dm-select` 220px×3(625-627 行);动态列表格 + `fixed="right"` 操作列(496-525 行);编辑弹窗 560px + `label-width="110px"`(532-533 行)。
- 改动:
  - scoped 媒体查询:`.card-actions { flex-wrap: wrap; row-gap: 8px }`、`.dm-select { width: 100% }`(`.default-model-row` 已有 wrap,619-623 行)。
  - 编辑弹窗:`:fullscreen="isMobile"` + label-position 响应式。
  - 表格:保持内部横向滚动,**保留 `fixed="right"` 操作列**——移动端滚动时操作按钮始终可见,是收益而非风险。

### 7. AgentImportModelsDialog.vue(从接口添加模型)
- 全项目最宽弹窗 640px(181-186 行);两个 `label-width="90px"` 表单;内嵌模型多选表(min≈422px,214-224 行)。
- 改动:`:fullscreen="isMobile"`;两个 form label-position 响应式;内嵌表格在全屏弹窗内可完整展示,无需额外处理;`.provider-url { float: right }`(284-289 行)窄屏拥挤属体验项,不动。

## 兼容性与回归

- 所有样式新规则都在 `@media (max-width: 768px)` 内、所有属性绑定都经 `isMobile` 判断,>768px 渲染结果与现状一致。
- MainLayout 是唯一结构重排点,用 `v-if/v-else` 隔离桌面分支。
- 恰好 768px 视口按移动端处理(与 EP `sm` 断点边界行为差异可接受)。

## 风险与权衡

- 全局 `.card-header/.toolbar` 选择器依赖现有类名约定:新增页面需沿用该命名,记入前端 spec(Phase 3.3)。
- `el-table` 移动端横向滚动体验一般,但避免重写为卡片列表,控制本次范围;列裁剪留待后续任务。
- fullscreen 弹窗在 768px 窄平板同样生效,视为可接受(全屏表单在平板竖屏同样好用)。

## 回滚

纯前端静态改动,单 commit 交付,回滚 = revert 该 commit。
