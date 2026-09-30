<script setup lang="ts">
// 表格单元格与编辑弹窗共用的受控字段输入组件:按列类型渲染对应的
// Element Plus 输入控件(readonly→纯文本、bool→开关、number→数字、
// list→多选、select→下拉、text→输入)。取值归一化内聚在此,父级只
// 接收归一化后的新值:number 清空发布 null(显式清除,后端移除对应
// 选项恢复缺省),list 发布字符串数组,bool 恒发布布尔值。
import type { AgentFieldSpec } from '@/types/agent'

defineProps<{ col: AgentFieldSpec; modelValue: unknown }>()

const emit = defineEmits<{ 'update:modelValue': [value: unknown] }>()

/** 布尔列取值:非布尔值(含缺失)一律按关闭展示。 */
function boolOf(v: unknown): boolean {
  return typeof v === 'boolean' ? v : false
}

/** 数字列取值:缺失或非法时返回 undefined,输入框留空。 */
function numOf(v: unknown): number | undefined {
  return typeof v === 'number' && Number.isFinite(v) ? v : undefined
}

/** 文本列取值:缺失或非字符串时按空串展示。 */
function strOf(v: unknown): string {
  return typeof v === 'string' ? v : ''
}

/** list 列取值:缺失或非数组时按空数组展示。 */
function listOf(v: unknown): string[] {
  return Array.isArray(v) ? v.map((item) => String(item)) : []
}

/** 只读列展示文本:布尔转是/否,数组转逗号分隔,空值显示占位符。 */
function cellText(v: unknown): string {
  if (v === undefined || v === null || v === '') return '—'
  if (typeof v === 'boolean') return v ? '是' : '否'
  if (Array.isArray(v)) {
    const text = v.map((item) => String(item)).join(', ')
    return text === '' ? '—' : text
  }
  return String(v)
}

/** 发布数字列新值;清空(null/undefined)发布 null=显式清除该选项,
 * 由父级纳入 patch、后端移除对应配置节点。 */
function emitNum(v: number | null | undefined): void {
  emit('update:modelValue', v ?? null)
}

/** 发布布尔列新值;开关默认只发布尔值。 */
function emitBool(v: string | number | boolean): void {
  emit('update:modelValue', v === true)
}

/** 发布 list 列新值(字符串数组)。 */
function emitList(v: unknown): void {
  emit('update:modelValue', listOf(v))
}
</script>

<template>
  <span v-if="col.readonly" class="readonly-cell">{{ cellText(modelValue) }}</span>
  <el-switch
    v-else-if="col.type === 'bool'"
    :model-value="boolOf(modelValue)"
    @update:model-value="emitBool"
  />
  <el-input-number
    v-else-if="col.type === 'number'"
    class="number-input"
    :controls="false"
    :model-value="numOf(modelValue)"
    @update:model-value="emitNum"
  />
  <el-select
    v-else-if="col.type === 'list'"
    class="cell-select"
    multiple
    filterable
    allow-create
    default-first-option
    :model-value="listOf(modelValue)"
    @update:model-value="emitList"
  >
    <el-option
      v-for="opt in col.options ?? []"
      :key="opt.value"
      :label="opt.label"
      :value="opt.value"
    />
  </el-select>
  <el-select
    v-else-if="col.type === 'select'"
    class="cell-select"
    clearable
    :model-value="strOf(modelValue)"
    @update:model-value="(v: string) => emit('update:modelValue', v)"
  >
    <el-option
      v-for="opt in col.options ?? []"
      :key="opt.value"
      :label="opt.label"
      :value="opt.value"
    />
  </el-select>
  <el-input
    v-else
    :model-value="strOf(modelValue)"
    @update:model-value="(v: string) => emit('update:modelValue', v)"
  />
</template>

<style scoped>
.readonly-cell {
  color: var(--el-text-color-regular);
  word-break: break-all;
}

.number-input,
.cell-select {
  width: 100%;
}
</style>
