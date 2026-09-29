<script setup lang="ts">
// Agent 模型配置卡片:展示单个 Agent 工具的配置现状与模型清单。
// 表格列由后端 Snapshot.Columns 驱动,行数据为本地编辑草稿;保存时仅收集
// 有改动的行与键,批量提交(后端写回前自动备份)。支持"默认模型"能力的工具
// (能力位 supports_default_model)额外渲染供应商/模型/档位三级选择与清除。
// 未找到配置时只展示中文指引,不渲染表格。请求统一走 useRequest,失败自动
// 弹中文提示。
import { computed, onMounted, ref, watch } from 'vue'

import {
  listAgentModels,
  restoreAgent,
  setDefaultAgentModel,
  updateAgentModels,
} from '@/api/agent'
import { useRequest } from '@/composables/useRequest'
import type {
  AgentDefaultModelPatch,
  AgentFieldSpec,
  AgentModelEntry,
  AgentModelPatch,
  AgentSnapshot,
} from '@/types/agent'

const props = defineProps<{ snapshot: AgentSnapshot }>()

// 还原/默认模型写回成功后配置状态可能变化,父级需刷新 Agent 列表
const emit = defineEmits<{ restored: []; defaultModelChanged: [] }>()

// 服务端最新清单与本地编辑草稿按下标配对,逐列比对生成增量补丁
const originals = ref<AgentModelEntry[]>([])
const drafts = ref<AgentModelEntry[]>([])

const {
  loading: modelsLoading,
  error: modelsError,
  run: runLoadModels,
} = useRequest(listAgentModels)
const { loading: saving, run: runSave } = useRequest(updateAgentModels)
const { loading: savingDefault, run: runSaveDefaultModel } = useRequest(setDefaultAgentModel)
const { loading: restoring, run: runRestore } = useRequest(restoreAgent)

const isFound = computed(() => props.snapshot.status === 'found')
// 只有非只读列可编辑,补丁收集也只针对这些列
const editableColumns = computed(() => props.snapshot.columns.filter((col) => !col.readonly))

// "默认模型"区编辑状态:随 Snapshot 同步(父级刷新后重置为服务端现状)
const defaultProvider = ref('')
const defaultModel = ref('')
const defaultLevel = ref('')

watch(
  () => props.snapshot,
  (snap) => {
    defaultProvider.value = snap.default_model?.provider_id ?? ''
    defaultModel.value = snap.default_model?.model_id ?? ''
    defaultLevel.value = snap.default_model?.reasoning_level ?? ''
  },
  { immediate: true },
)

const hasDefaultModel = computed(() => props.snapshot.default_model != null)

/** 默认模型的供应商候选:模型清单条目按 provider_id 去重。 */
const defaultProviderOptions = computed(() => {
  const seen = new Set<string>()
  const options: string[] = []
  for (const entry of originals.value) {
    const pid = entry.provider_id ?? ''
    if (pid !== '' && !seen.has(pid)) {
      seen.add(pid)
      options.push(pid)
    }
  }
  return options
})

/** 默认模型的模型候选:当前供应商下的条目。 */
const defaultModelOptions = computed(() =>
  originals.value
    .filter((entry) => (entry.provider_id ?? '') === defaultProvider.value)
    .map((entry) => entry.model_id),
)

/** 默认模型的档位候选:所选条目规则定义的 reasoning_levels。 */
const defaultLevelOptions = computed(() => {
  const entry = originals.value.find(
    (item) =>
      (item.provider_id ?? '') === defaultProvider.value && item.model_id === defaultModel.value,
  )
  const levels = entry?.fields['reasoning_levels']
  return Array.isArray(levels) ? levels.map((level) => String(level)) : []
})

/** 切换供应商后模型与档位随之失效,重置避免提交脏组合。 */
function onDefaultProviderChange() {
  defaultModel.value = ''
  defaultLevel.value = ''
}

/** 切换模型后档位候选变化,重置档位。 */
function onDefaultModelChange() {
  defaultLevel.value = ''
}

/** 保存默认模型:提交设置 patch,成功后提示写回生效方式并通知父级刷新。 */
async function handleSaveDefaultModel() {
  if (defaultProvider.value === '' || defaultModel.value === '') {
    ElMessage.warning('请先选择供应商与模型')
    return
  }
  const patch: AgentDefaultModelPatch = {
    provider_id: defaultProvider.value,
    model_id: defaultModel.value,
  }
  if (defaultLevel.value !== '') patch.reasoning_level = defaultLevel.value
  const snap = await runSaveDefaultModel(props.snapshot.name, patch)
  if (snap === undefined) return // 失败已由 useRequest 统一提示
  ElMessage.success(`已写回,建议重启 ${props.snapshot.display_name} 使配置生效`)
  emit('defaultModelChanged')
}

/** 清除默认模型:二次确认后提交全空 patch,成功后提示写回生效方式并通知父级刷新。 */
async function handleClearDefaultModel() {
  try {
    await ElMessageBox.confirm(
      `确定清除「${props.snapshot.display_name}」的默认模型吗?`,
      '清除确认',
      { type: 'warning', confirmButtonText: '清除', cancelButtonText: '取消' },
    )
  } catch {
    return // 用户取消,无需提示
  }
  const snap = await runSaveDefaultModel(props.snapshot.name, { provider_id: '', model_id: '' })
  if (snap === undefined) return
  ElMessage.success(`已写回,建议重启 ${props.snapshot.display_name} 使配置生效`)
  emit('defaultModelChanged')
}

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

/** list 列取值:缺失或非数组时按空数组展示。 */
function listOf(v: unknown): string[] {
  return Array.isArray(v) ? v.map((item) => String(item)) : []
}

/** 写回 list 列(字符串数组)。行不存在时忽略。 */
function setList(index: number, key: string, v: string[]): void {
  const row = drafts.value[index]
  if (!row) return
  row.fields[key] = v
}

/** 按列类型给最小列宽:开关窄,数字与下拉适中,list 与文本较宽。 */
function colWidth(col: AgentFieldSpec): number {
  switch (col.type) {
    case 'bool':
      return 90
    case 'number':
    case 'select':
      return 150
    case 'list':
      return 220
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
      <!-- 默认模型:仅支持该能力的工具展示,三级选项来自模型清单且均可自由输入 -->
      <div v-if="snapshot.supports_default_model" class="default-model">
        <div class="default-model-header">
          <span class="default-model-title">默认模型</span>
          <span class="default-model-current">
            <template v-if="hasDefaultModel">
              当前:{{ snapshot.default_model?.provider_id }} /
              {{ snapshot.default_model?.model_id
              }}{{
                snapshot.default_model?.reasoning_level
                  ? ` / ${snapshot.default_model.reasoning_level}`
                  : ''
              }}
            </template>
            <template v-else>当前:未设置</template>
          </span>
        </div>
        <div class="default-model-row">
          <el-select
            v-model="defaultProvider"
            class="dm-select"
            placeholder="供应商"
            filterable
            allow-create
            default-first-option
            @change="onDefaultProviderChange"
          >
            <el-option v-for="p in defaultProviderOptions" :key="p" :label="p" :value="p" />
          </el-select>
          <el-select
            v-model="defaultModel"
            class="dm-select"
            placeholder="模型"
            filterable
            allow-create
            default-first-option
            @change="onDefaultModelChange"
          >
            <el-option v-for="m in defaultModelOptions" :key="m" :label="m" :value="m" />
          </el-select>
          <el-select
            v-model="defaultLevel"
            class="dm-select"
            placeholder="推理档位(可选)"
            filterable
            allow-create
            default-first-option
            clearable
          >
            <el-option v-for="lv in defaultLevelOptions" :key="lv" :label="lv" :value="lv" />
          </el-select>
          <el-button type="primary" :loading="savingDefault" @click="handleSaveDefaultModel">
            保存
          </el-button>
          <el-button
            v-if="hasDefaultModel"
            :loading="savingDefault"
            @click="handleClearDefaultModel"
          >
            清除
          </el-button>
        </div>
      </div>
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
              v-else-if="col.type === 'list'"
              class="cell-select"
              multiple
              filterable
              allow-create
              default-first-option
              :model-value="listOf(cellValue($index, col.key))"
              @update:model-value="(v) => setList($index, col.key, listOf(v))"
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

.default-model {
  margin-top: 12px;
  margin-bottom: 12px;
}

.default-model-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
}

.default-model-title {
  font-weight: 600;
}

.default-model-current {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.default-model-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.dm-select {
  width: 220px;
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
