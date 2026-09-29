<script setup lang="ts">
// 后台整体布局:左侧品牌与菜单 + 顶栏说明 + 主内容区
import { useRoute } from 'vue-router'

const route = useRoute()
</script>

<template>
  <el-container class="layout">
    <el-aside width="220px" class="aside">
      <div class="brand">ModelMeter</div>
      <el-menu router :default-active="route.path">
        <el-menu-item index="/">首页</el-menu-item>
        <el-menu-item index="/providers">接口配置</el-menu-item>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header class="header">LLM 接口管理与模型测试工作台</el-header>
      <el-main class="main">
        <RouterView />
      </el-main>
    </el-container>
  </el-container>
</template>

<style scoped>
.layout {
  height: 100%;
}

.aside {
  border-right: 1px solid var(--el-border-color-light);
}

.brand {
  padding: 20px 16px;
  font-size: 18px;
  font-weight: 600;
  color: var(--el-color-primary);
}

.header {
  display: flex;
  align-items: center;
  border-bottom: 1px solid var(--el-border-color-light);
  color: var(--el-text-color-secondary);
}

.main {
  background: var(--el-fill-color-lighter);
}
</style>
