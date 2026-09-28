# 类型安全

> 本项目的 TypeScript 约定。

---

## 概述

- `tsconfig.json` 开启 `"strict": true`,不关闭任何 strict 子项。
- 前后端数据边界(即后端统一 JSON 信封)集中定义在 `src/types/api.ts`,
  各资源的请求/响应类型与接口封装一起放在 `src/api/*.ts`。
- 运行时校验暂不引入 Zod 等库:后端响应结构由类型约定保证;类型只保证
  编译期正确,不可靠字段用运行时判断收窄。

---

## 类型组织

```ts
// src/types/api.ts —— 与后端 error-handling.md 的统一信封对应
export interface ApiResponse<T> {
  code: number      // 0 成功,非 0 为业务错误码
  message: string   // 中文提示
  data: T | null
}

// src/api/meter.ts —— 资源类型与接口同文件
export interface Meter {
  id: number
  name: string
  unit: string
  createdAt: string   // 后端时间为 UTC ISO 字符串
}

export function listMeters(params: MeterQuery): Promise<ApiResponse<Meter[]>> { ... }
```

- 组件 props / emits 类型就近写在组件里,跨组件复用的领域类型进
  `src/api/` 或 `src/types/`。
- 后端新增字段时,先更新 `api/` 里的类型再写页面。

---

## 校验

- 用户输入校验用 Element Plus 表单的 `rules`(中文提示文案)。
- 对"不可信来源"(localStorage、URL query)的值先做运行时判断再按类型
  使用,不要直接断言。

---

## 常用模式

- 联合类型表达互斥状态:`type SaveState = 'idle' | 'saving' | 'saved' | 'error'`。
- 从值推导类型:`typeof` / `as const` 用于常量表。
- 收窄用类型守卫函数(`function isMeter(x: unknown): x is Meter`)。

---

## 禁止模式

- `any`(含隐式 any);确实未知类型用 `unknown` + 收窄。
- 双重断言 `as unknown as T`。
- `@ts-ignore`;确需豁免用 `@ts-expect-error` 并附中文注释说明原因。
- 非空断言 `!` 用于可能为空的运行时值(仅限"构造上不可能为空"的场合)。
- `as` 断言仅限两类边界:DOM API、没有类型定义的第三方库;业务数据
  禁止断言,应修改类型定义。
