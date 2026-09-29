// 应用入口:装配 Pinia、路由与全局样式后挂载。
// Element Plus 组件由 unplugin 插件按需自动导入,无需全量注册。
import { createApp } from 'vue'
import { createPinia } from 'pinia'

import App from './App.vue'
import router from './router'
import '@/styles/index.css'

const app = createApp(App)

app.use(createPinia())
app.use(router)

app.mount('#app')
