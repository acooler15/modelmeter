<script setup lang="ts">
// 应用根组件:用 ElConfigProvider 注入 Element Plus 中文语言包(组件按需自动
// 导入,内置文案默认英文,如空数据/分页),布局由各页面使用的 Layout 组件负责
import zhCn from 'element-plus/es/locale/lang/zh-cn'
</script>

<template>
  <el-config-provider :locale="zhCn">
    <RouterView />
  </el-config-provider>
</template>
