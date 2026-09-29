<script setup lang="ts">
// Agent 模型配置卡片:展示单个 Agent 工具的配置现状与模型清单。
// 表格列由后端 Snapshot.Columns 驱动,行数据为本地编辑草稿;保存时仅收集
// 有改动的行与键,批量提交(后端写回前自动备份)。未找到配置时只展示
// 中文指引,不渲染表格。请求统一走 useRequest,失败自动弹中文提示。
import { computed, onMounted, ref } from 'vue'

import { listAgentModels, restoreAgent, updateAgentModels } from '@/api/agent'
import { useRequest } from '@/composables/useRequest'
import type { AgentFieldSpec, AgentModelEntry, AgentModelPatch, AgentSnapshot } from '@/types/agent'

const props = defineProps<{ snapshot: AgentSnapshot }>()

// 还原成功后配置可能从"未找到"变为"已找到",父级需刷新 Agent 列表
const emit = defineEmits<{ restored: [] }>()

// 服务端最新清单与本地编辑草稿按下标配对,逐列比对生成增量补丁
const originals = ref<AgentModelEntry[]>([])
const drafts = ref<AgentModelEntry[]>([])

const {
  loading: modelsLoading,
  error: modelsError,
  run: runLoadModels,
} = useRequest(listAgentModels)
const { loading: saving, run: runSave } = useRequest(updateAgentModels)
const { loading: restoring, run: runRestore } = useRequest(restoreAgent)

const isFound = computed(() => props.snapshot.status === 'found')
// 只有非只读列可编辑,补丁收集也只针对这些列
const editableColumns = computed(() => props.snapshot.columns.filter((col) => !col.readonly))

/** 用服务端清单替换本地数据并重置草稿。 */
function applyEntries(list: AgentModelEntry[]) {
  originals.value = list
  drafts.value = list.map((entry) => ({ ...entry, fields: { ...entry.fields } }))
}

/** 拉取模型清单;失败已由 useRequest 统一提示。 */
async function loadModels() {
  const list = await runLoadModels(props.snapshot.name)
  if (list === undefined) return
  applyEntries(list)
}

/** 判断草稿值相对原值是否为有效改动;清空(undefined/null)视为放弃修改。 */
function valueChanged(orig: unknown, draft: unknown): boolean {
  if (draft === undefined || draft === null) return false
  // 原本为空的文本列被清空不算改动,避免把空串写回原无此键的条目
  if (draft === '' && (orig === undefined || orig === null || orig === '')) return false
  if (Array.isArray(orig) || Array.isArray(draft)) {
    return JSON.stringify(draft) !== JSON.stringify(orig)
  }
  return draft !== orig
}

/** 逐行逐列比对草稿与原值,只收集有改动的行与键,生成增量补丁。 */
const changedPatches = computed<AgentModelPatch[]>(() => {
  const patches: AgentModelPatch[] = []
  drafts.value.forEach((draft, index) => {
    const orig = originals.value[index]
    if (!orig) return
    const fields: Record<string, unknown> = {}
    for (const col of editableColumns.value) {
      if (valueChanged(orig.fields[col.key], draft.fields[col.key])) {
        fields[col.key] = draft.fields[col.key]
      }
    }
    if (Object.keys(fields).length > 0) {
      // provider_id 仅 ZCode 存在;序列化时会自动省略 undefined
      patches.push({ provider_id: orig.provider_id, model_id: orig.model_id, fields })
    }
  })
  return patches
})
const hasChanges = computed(() => changedPatches.value.length > 0)

/** 批量保存:提交全部增量补丁,成功后用响应的最新清单重置草稿。 */
async function handleSave() {
  if (!hasChanges.value) return
  const list = await runSave(props.snapshot.name, changedPatches.value)
  if (list === undefined) return // 失败已由 useRequest 统一提示
  applyEntries(list)
  ElMessage.success('已写回,建议重启对应工具使配置生效')
}

/** 一键还原:二次确认后从最近一份备份还原,并刷新清单与配置状态。 */
async function handleRestore() {
  if (!props.snapshot.config_path) return
  try {
    await ElMessageBox.confirm(
      `确定将「${props.snapshot.display_name}」的配置还原到最近一份备份吗?`,
      '还原确认',
      { type: 'warning', confirmButtonText: '还原', cancelButtonText: '取消' },
    )
  } catch {
    return // 用户取消,无需提示
  }
  const snap = await runRestore(props.snapshot.name)
  if (snap === undefined) return
  ElMessage.success('已从最近一份备份还原')
  emit('restored')
  // 还原后配置文件可能从无到有,重新拉取模型清单
  if (snap.status === 'found') void loadModels()
}

/** 取草稿行某列的原始值;行不存在时返回 undefined。 */
function cellValue(index: number, key: string): unknown {
  return drafts.value[index]?.fields[key]
}

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

/** 写回布尔列;开关默认只发布尔值。行不存在时忽略。 */
function setBool(index: number, key: string, v: string | number | boolean): void {
  const row = drafts.value[index]
  if (!row) return
  row.fields[key] = v === true
}

/** 写回数字列;清空(null/undefined)等价于放弃该列修改。行不存在时忽略。 */
function setNum(index: number, key: string, v: number | null | undefined): void {
  const row = drafts.value[index]
  if (!row) return
  if (v === null || v === undefined) {
    delete row.fields[key]
    return
  }
  row.fields[key] = v
}

/** 写回文本/下拉列。行不存在时忽略。 */
function setStr(index: number, key: string, v: string): void {
  const row = drafts.value[index]
  if (!row) return
  row.fields[key] = v
}

/** 按列类型给最小列宽:开关窄,数字与下拉适中,文本较宽。 */
function colWidth(col: AgentFieldSpec): number {
  switch (col.type) {
    case 'bool':
      return 90
    case 'number':
    case 'select':
      return 150
    default:
      return 180
  }
}

// 已找到配置的卡片挂载即拉取模型清单
onMounted(() => {
  if (isFound.value) void loadModels()
})
</script>

<template>
  <el-card class="agent-card" shadow="never">
    <template #header>
      <div class="card-header">
        <div class="card-title">
          <span class="agent-name">{{ snapshot.display_name }}</span>
          <el-tag :type="isFound ? 'success' : 'info'">
            {{ isFound ? '已找到' : '未找到' }}
          </el-tag>
        </div>
        <div class="card-actions">
          <el-button type="primary" :disabled="!hasChanges" :loading="saving" @click="handleSave">
            保存修改
          </el-button>
          <el-button
            :disabled="!snapshot.config_path"
            :loading="restoring"
            @click="handleRestore"
          >
            还原上次备份
          </el-button>
        </div>
      </div>
    </template>

    <div class="config-path">配置文件:{{ snapshot.config_path || '未知' }}</div>

    <el-alert
      v-if="snapshot.message"
      class="agent-message"
      :title="snapshot.message"
      :type="isFound ? 'info' : 'warning'"
      show-icon
      :closable="false"
    />

    <!-- 未找到配置:只展示指引,不渲染表格 -->
    <el-empty v-if="!isFound" description="未找到配置文件,请按上方指引确认工具已安装并运行过一次" />
    <template v-else>
      <el-alert
        v-if="modelsError"
        class="models-error"
        type="error"
        :title="modelsError.message"
        show-icon
        :closable="false"
      />
      <el-table v-loading="modelsLoading" :data="drafts" empty-text="该工具暂无模型条目">
        <el-table-column
          v-for="col in snapshot.columns"
          :key="col.key"
          :label="col.label"
          :min-width="colWidth(col)"
        >
          <template #default="{ $index }">
            <span v-if="col.readonly" class="readonly-cell">{{ cellText(cellValue($index, col.key)) }}</span>
            <el-switch
              v-else-if="col.type === 'bool'"
              :model-value="boolOf(cellValue($index, col.key))"
              @update:model-value="(v) => setBool($index, col.key, v)"
            />
            <el-input-number
              v-else-if="col.type === 'number'"
              class="number-input"
              :controls="false"
              :model-value="numOf(cellValue($index, col.key))"
              @update:model-value="(v) => setNum($index, col.key, v)"
            />
            <el-select
              v-else-if="col.type === 'select'"
              class="cell-select"
              clearable
              :model-value="strOf(cellValue($index, col.key))"
              @update:model-value="(v) => setStr($index, col.key, v)"
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
              :model-value="strOf(cellValue($index, col.key))"
              @update:model-value="(v) => setStr($index, col.key, v)"
            />
          </template>
        </el-table-column>
      </el-table>
      <div v-if="hasChanges" class="dirty-hint">
        有 {{ changedPatches.length }} 行修改未保存;保存前会自动备份原配置
      </div>
    </template>
  </el-card>
</template>

<style scoped>
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.card-title {
  display: flex;
  align-items: center;
  gap: 8px;
}

.agent-name {
  font-size: 16px;
  font-weight: 600;
}

.card-actions {
  display: flex;
  gap: 0;
}

.config-path {
  font-size: 13px;
  color: var(--el-text-color-secondary);
  word-break: break-all;
}

.agent-message {
  margin-top: 12px;
}

.models-error {
  margin-top: 12px;
}

.readonly-cell {
  color: var(--el-text-color-regular);
  word-break: break-all;
}

.number-input,
.cell-select {
  width: 100%;
}

.dirty-hint {
  margin-top: 12px;
  font-size: 13px;
  color: var(--el-color-warning);
}
</style>
