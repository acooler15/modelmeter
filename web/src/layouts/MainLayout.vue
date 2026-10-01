<script setup lang="ts">
// 后台整体布局:桌面端为左侧品牌菜单 + 顶栏说明 + 主内容区(与历史结构一致);
// 窄屏(≤768px)下侧边栏收起,顶栏提供纯 CSS 汉堡按钮与抽屉式菜单,
// 点击菜单项跳转后抽屉自动收起。断点与 useIsMobile 保持一致。
import { ref, watch } from 'vue'
import { useRoute } from 'vue-router'

import { useIsMobile } from '@/composables/useIsMobile'

const route = useRoute()
const isMobile = useIsMobile()

// 菜单项唯一定义:桌面侧边栏与移动抽屉共用 v-for,避免双份维护
const menuItems = [
  { index: '/', label: '首页' },
  { index: '/providers', label: '接口配置' },
  { index: '/models', label: '模型列表' },
  { index: '/test', label: '模型测试' },
  { index: '/rates', label: '模型费率' },
  { index: '/agents', label: 'Agent 配置' },
]

// 移动端抽屉开关;路由变化(点击菜单项跳转)后自动收起
const menuOpen = ref(false)

watch(
  () => route.path,
  () => {
    menuOpen.value = false
  },
)
</script>

<template>
  <el-container class="layout">
    <!-- 桌面端:保持既有左侧边栏结构 -->
    <el-aside v-if="!isMobile" width="220px" class="aside">
      <div class="brand">ModelMeter</div>
      <el-menu router :default-active="route.path">
        <el-menu-item v-for="item in menuItems" :key="item.index" :index="item.index">
          {{ item.label }}
        </el-menu-item>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header class="header">
        <!-- 移动端:汉堡按钮(三条横线,不用图标包)+ 品牌 -->
        <template v-if="isMobile">
          <div class="mobile-header">
            <button
              type="button"
              class="hamburger"
              aria-label="打开导航菜单"
              :aria-expanded="menuOpen"
              @click="menuOpen = !menuOpen"
            >
              <span></span>
              <span></span>
              <span></span>
            </button>
            <span class="brand mobile-brand">ModelMeter</span>
          </div>
        </template>
        <!-- 桌面端:顶栏保持纯文案 -->
        <template v-else>LLM 接口管理与模型测试工作台</template>
      </el-header>
      <el-main class="main">
        <RouterView />
      </el-main>
    </el-container>

    <!-- 移动端抽屉菜单;桌面端不渲染,保持 DOM 与现状一致 -->
    <el-drawer
      v-if="isMobile"
      v-model="menuOpen"
      direction="ltr"
      size="240px"
      :with-header="false"
    >
      <div class="brand drawer-brand">ModelMeter</div>
      <!-- select 事件兜底:点击当前路由菜单项时 path 不变、watch 不触发,仍需收起抽屉 -->
      <el-menu
        router
        :default-active="route.path"
        class="drawer-menu"
        @select="menuOpen = false"
      >
        <el-menu-item v-for="item in menuItems" :key="item.index" :index="item.index">
          {{ item.label }}
        </el-menu-item>
      </el-menu>
    </el-drawer>
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

/* 移动端顶栏:汉堡按钮 + 品牌横排 */
.mobile-header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.mobile-brand {
  padding: 0;
  font-size: 17px;
}

/* 纯 CSS 汉堡按钮:三条横线,不用图标包 */
.hamburger {
  display: inline-flex;
  flex-direction: column;
  justify-content: center;
  gap: 4px;
  width: 36px;
  height: 36px;
  padding: 9px 8px;
  border: none;
  border-radius: 4px;
  background: transparent;
  cursor: pointer;
}

.hamburger span {
  display: block;
  width: 100%;
  height: 2px;
  border-radius: 1px;
  background: var(--el-text-color-primary);
}

.hamburger:hover {
  background: var(--el-fill-color);
}

/* 抽屉内品牌与菜单:压缩侧边距适配 240px 宽度 */
.drawer-brand {
  padding: 16px;
  font-size: 17px;
}

.drawer-menu {
  border-right: none;
}
</style>
