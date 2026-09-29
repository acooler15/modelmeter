/// <reference types="vite/client" />

export {}

// 路由元信息类型增强:避免在业务代码里对 meta 做类型断言
declare module 'vue-router' {
  interface RouteMeta {
    /** 页面标题,用于浏览器标签页文案 */
    title?: string
  }
}
