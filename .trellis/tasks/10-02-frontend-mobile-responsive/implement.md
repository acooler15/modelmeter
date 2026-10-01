# 执行计划:前端移动端适配

按序执行;每步独立可验证,整体单 commit 交付(回滚 = revert)。

## 前置

- [ ] 阅读规范:`.trellis/spec/frontend/` 下 component-guidelines.md、composable-guidelines.md、directory-structure.md、quality-guidelines.md

## 实施步骤

1. [ ] 新增 `web/src/composables/useIsMobile.ts`:matchMedia 单例(见 design.md 代码),返回只读 `isMobile`
2. [ ] 改造 `web/src/layouts/MainLayout.vue`:
   - `menuItems` 常量数组 + 两处 `v-for` 共用
   - `isMobile` 分支:桌面 aside 结构不变;移动端 header 放 CSS 汉堡按钮(纯 span,不用图标包)+ 品牌,`el-drawer` 承载菜单
   - `watch(route.path)` 关闭抽屉
3. [ ] `web/src/styles/index.css` 追加 `@media (max-width: 768px)` 块:`.el-main`/`.el-card__body` 内边距 12px、`.card-header`/`.toolbar` 换行、`.el-radio-group` 换行、`.el-message-box` 宽度保护
4. [ ] `web/src/views/provider/ProviderList.vue`:dialog `:fullscreen="isMobile"`,form label-position 响应式
5. [ ] `web/src/views/model/ModelListView.vue`:scoped 媒体查询,`.provider-select`/`.keyword-input` 窄屏 100% 宽、取消 `margin-left: auto`
6. [ ] `web/src/views/test/ModelTestView.vue`:两处 form label-position 响应式;scoped 媒体查询 `.provider-select` 100% 宽;结果 descriptions `:column="isMobile ? 1 : 3"`;详情弹窗 `:fullscreen` + 内部 descriptions `:column="isMobile ? 1 : 2"`
7. [ ] `web/src/views/rate/RateView.vue`:form label-position 响应式;scoped 媒体查询 `.filter-input` 100% 宽;`.configured-hint` 加 `word-break: break-all`
8. [ ] `web/src/components/agent/AgentModelCard.vue`:scoped 媒体查询 `.card-actions` 换行、`.dm-select` 100% 宽;编辑弹窗 `:fullscreen` + label-position 响应式;保留表格 `fixed="right"` 操作列
9. [ ] `web/src/components/agent/AgentImportModelsDialog.vue`:dialog `:fullscreen`,两个 form label-position 响应式
10. [ ] 核对 `web/src/views/agent/AgentView.vue`:确认仅依赖全局规则即可,无遗漏自有固定宽度

## 验证

- [ ] `cd web && npm run lint` 通过
- [ ] `cd web && npm run build`(vue-tsc --noEmit + vite build)通过
- [ ] 启动服务(后端或 `npm run dev`),DevTools 375px 视口逐页冒烟:六页无页面级横向滚动、抽屉导航可用、4 个弹窗全屏可用、descriptions 1 列;≥1024px 视口回归桌面布局不变(对照 git 现状)

## 评审门

- 步骤 1-3 完成后先自查全局规则是否影响桌面端,再继续页面级改动
- 全部完成后走 trellis-check 质量检查,再进入 Phase 3
