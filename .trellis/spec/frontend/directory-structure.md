# 目录结构

> 前端(Vue 3)代码的组织方式。

---

## 概述

- **框架**:Vue 3(`<script setup>` 组合式 API)+ TypeScript。
- **构建**:Vite;产物输出到 `web/dist`,由 Go 后端 `go:embed` 嵌入单二进制。
- **UI 库**:Element Plus(按需自动导入)。
- **路由**:`vue-router`(history 模式,后端需配置 SPA 回退)。
- **状态**:Pinia。

> 约定:代码标识符用英文,**注释一律用中文**。

---

## 目录布局

```
web/
├── src/
│   ├── api/              # 按资源拆分的后端接口封装(meter.ts),含请求/响应类型
│   ├── components/       # 通用组件,可复用、与业务页面解耦
│   ├── composables/      # 组合式函数 useXxx.ts(见 composable-guidelines.md)
│   ├── layouts/          # 布局组件(侧边栏 + 顶栏的后台框架)
│   ├── views/            # 路由页面组件,与 router 一一对应
│   ├── router/           # 路由表 index.ts,懒加载 views
│   ├── stores/           # Pinia store,一个领域一个文件(user.ts)
│   ├── styles/           # 全局样式、Element Plus 主题变量覆盖
│   ├── types/            # 跨模块共享的 TS 类型(api.ts 定义统一信封)
│   ├── utils/            # 纯函数工具,不放业务状态
│   ├── App.vue
│   └── main.ts           # 装配:Pinia、router、Element Plus、全局样式
├── index.html
├── vite.config.ts        # dev 代理 /api → localhost:8422;build.outDir 默认 dist
├── tsconfig.json         # strict: true
└── package.json
```

---

## 模块划分

- **页面即模块**:一个路由页面对应 `views/` 下一个组件;该页面独有、且
  暂无复用需求的子组件,先内联在页面文件里,出现第二次使用再提升到
  `components/`。
- **接口层**:组件不直接 `fetch`/`axios`,一律调用 `api/` 里的封装函数;
  请求与响应类型和函数写在同一文件。
- **跨页面共享的服务端数据**才进 Pinia store;页面私有数据留在页面组件
  或 composable 里。
- 新增资源(如 `meter`)的纵向切片:
  `api/meter.ts` → `views/meter/MeterList.vue` → 路由表加一行 →
  需要共享时补 `stores/meter.ts`。

---

## 命名约定

- 组件文件:PascalCase(`MeterList.vue`),组件名至少两个单词,避免与
  原生 HTML 标签冲突。
- composable:`useXxx.ts`,与函数同名。
- 其余 ts 文件:camelCase(`formatTime.ts`)。
- 目录名:kebab-case 或单词(`meter-list/`),页面目录与其路由 path 对应。
- 模板中组件用 PascalCase 标签(`<MeterList />`),Element Plus 组件保持
  官方 kebab-case 或 PascalCase 均可,同一文件内保持一致。

---

## 示例

第一个纵向切片(计量表列表页)落地后即为参考模式;后续页面照搬其
api → view → router 的接法。
