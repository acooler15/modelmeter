// 路由表:history 模式,页面组件懒加载;后端已配置 SPA 回退到 index.html
import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      component: () => import('@/layouts/MainLayout.vue'),
      children: [
        {
          path: '',
          name: 'home',
          component: () => import('@/views/home/HomeView.vue'),
          meta: { title: '首页' },
        },
      ],
    },
    // 未匹配路径统一回到首页
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

// 路由切换后同步页面标题,文案保持中文
router.afterEach((to) => {
  const title = to.meta.title
  document.title = title ? `${title} - ModelMeter` : 'ModelMeter'
})

export default router
