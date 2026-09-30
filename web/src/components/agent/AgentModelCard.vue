<script setup lang="ts">
// Agent 模型配置卡片:展示单个 Agent 工具的配置现状与模型清单。
// 表格列由后端 Snapshot.Columns 驱动,行数据为本地编辑草稿;保存时仅收集
// 有改动的行与键,批量提交(后端写回前自动备份)。单元格与编辑弹窗共用
// AgentFieldInput 受控组件。支持删除能力的工具(能力位 supports_remove_models)
// 额外提供多选批量删除与单行删除(一次请求一次备份一次写回);支持添加能力
// 的工具(supports_add_models)提供"从接口添加模型"弹窗。支持"默认模型"
// 能力的工具(supports_default_model)额外渲染供应商/模型/档位三级选择与清除。
// 未找到配置时只展示中文指引,不渲染表格与增删改入口。请求统一走 useRequest,
// 失败自动弹中文提示。
import { computed, onMounted, ref, watch } from 'vue'

import AgentFieldInput from '@/components/agent/AgentFieldInput.vue'
import AgentImportModelsDialog from '@/components/agent/AgentImportModelsDialog.vue'
import {
  listAgentModels,
  removeAgentModels,
  restoreAgent,
  setDefaultAgentModel,
  updateAgentModels,
} from '@/api/agent'
import { useRequest } from '@/composables/useRequest'
import type {
  AgentAddModelsResult,
  AgentDefaultModelPatch,
  AgentFieldSpec,
  AgentModelEntry,
  AgentModelPatch,
  AgentModelRef,
  AgentSnapshot,
} from '@/types/agent'

const props = defineProps<{ snapshot: AgentSnapshot }>()

// 写回成功后配置状态可能变化,父级需刷新 Agent 列表;providerCreated 表示
// ZCode 新建了供应商(落点候选 add_targets 已变化),同样需要父级重拉列表
const emit = defineEmits<{ restored: []; defaultModelChanged: []; providerCreated: [] }>()

// 服务端最新清单与本地编辑草稿按下标配对,逐列比对生成增量补丁
const originals = ref<AgentModelEntry[]>([])
const drafts = ref<AgentModelEntry[]>([])

const {
  loading: modelsLoading,
  error: modelsError,
  run: runLoadModels,
} = useRequest(listAgentModels)
const { loading: saving, run: runSave } = useRequest(updateAgentModels)
const { loading: removing, run: runRemoveModels } = useRequest(removeAgentModels)
const { loading: savingDefault, run: runSaveDefaultModel } = useRequest(setDefaultAgentModel)
const { loading: restoring, run: runRestore } = useRequest(restoreAgent)

const isFound = computed(() => props.snapshot.status === 'found')
// 只有非只读列可编辑,补丁收集也只针对这些列
const editableColumns = computed(() => props.snapshot.columns.filter((col) => !col.readonly))
// 删除/添加入口按能力位 + 配置已找到双重控制
const canRemoveModels = computed(() => isFound.value && props.snapshot.supports_remove_models)
const canAddModels = computed(() => isFound.value && props.snapshot.supports_add_models)

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

// 表格多选:选中的行为草稿对象,自带定位键(provider_id/model_id 不可编辑)
const selectedRows = ref<AgentModelEntry[]>([])

function onSelectionChange(rows: AgentModelEntry[]) {
  selectedRows.value = rows
}

/** 用服务端清单替换本地数据并重置草稿与多选。 */
function applyEntries(list: AgentModelEntry[]) {
  originals.value = list
  drafts.value = list.map((entry) => ({ ...entry, fields: { ...entry.fields } }))
  selectedRows.value = []
}

/** 拉取模型清单;失败已由 useRequest 统一提示。 */
async function loadModels() {
  const list = await runLoadModels(props.snapshot.name)
  if (list === undefined) return
  applyEntries(list)
}

/**
 * 判断草稿值相对原值是否为有效改动。draft undefined=放弃修改;
 * draft null=显式清除(仅原值存在时构成改动,patch 携带 null 由后端移除
 * 对应选项节点恢复工具缺省);原本为空的文本列被清空不算改动。
 */
function valueChanged(orig: unknown, draft: unknown): boolean {
  if (draft === undefined) return false
  if (draft === null) return orig !== undefined && orig !== null
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

/** 取草稿行某列的值;行不存在时返回 undefined。 */
function cellValue(index: number, key: string): unknown {
  return drafts.value[index]?.fields[key]
}

/** 写回单元格;undefined 等价于放弃该列修改,null 保留进草稿(显式清除
 * 语义,保存时随 patch 提交)。行不存在时忽略。 */
function setCell(index: number, key: string, v: unknown): void {
  const row = drafts.value[index]
  if (!row) return
  if (v === undefined) {
    delete row.fields[key]
    return
  }
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

// 删除通道:单行删除与批量删除复用,一次请求一次备份一次写回
async function handleRemoveModels(refs: AgentModelRef[]) {
  if (refs.length === 0) return
  try {
    await ElMessageBox.confirm(
      `确定删除选中的 ${refs.length} 个模型吗?删除前会自动备份原配置。`,
      '删除确认',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
  } catch {
    return // 用户取消,无需提示
  }
  const list = await runRemoveModels(props.snapshot.name, refs)
  if (list === undefined) return
  applyEntries(list)
  ElMessage.success(`已删除 ${refs.length} 个模型,建议重启对应工具使配置生效`)
}

/** 单行删除入口。 */
function handleRemoveRow(index: number) {
  const orig = originals.value[index]
  if (!orig) return
  void handleRemoveModels([{ provider_id: orig.provider_id, model_id: orig.model_id }])
}

/** 批量删除入口:选中的行携带定位键。 */
function handleRemoveSelected() {
  const refs = selectedRows.value.map((row) => ({
    provider_id: row.provider_id,
    model_id: row.model_id,
  }))
  void handleRemoveModels(refs)
}

// 编辑弹窗:以该行当前草稿值初始化(含未保存的行内修改),确定即提交单行
// patch 并用响应整体刷新,避免弹窗与行内两套脏状态叠加
const editVisible = ref(false)
const editIndex = ref(-1)
const editForm = ref<Record<string, unknown>>({})

/** 打开编辑弹窗。 */
function openEditDialog(index: number) {
  const draft = drafts.value[index]
  if (!draft) return
  editIndex.value = index
  editForm.value = { ...draft.fields }
  editVisible.value = true
}

/** 写回编辑弹窗表单;undefined 等价于放弃该列修改,null 保留(显式清除语义)。 */
function setEditField(key: string, v: unknown): void {
  if (v === undefined) {
    delete editForm.value[key]
    return
  }
  editForm.value[key] = v
}

/** 弹窗内是否有相对服务端原值的有效改动。 */
const hasEditChanges = computed(() => {
  const orig = originals.value[editIndex.value]
  if (!orig) return false
  return editableColumns.value.some((col) =>
    valueChanged(orig.fields[col.key], editForm.value[col.key]),
  )
})

/** 提交编辑弹窗:只收集该行有改动的键,生成单行 patch 走白名单写回通道。 */
async function handleEditSubmit() {
  const orig = originals.value[editIndex.value]
  if (!orig) return
  const fields: Record<string, unknown> = {}
  for (const col of editableColumns.value) {
    if (valueChanged(orig.fields[col.key], editForm.value[col.key])) {
      fields[col.key] = editForm.value[col.key]
    }
  }
  const list = await runSave(props.snapshot.name, [
    { provider_id: orig.provider_id, model_id: orig.model_id, fields },
  ])
  if (list === undefined) return // 失败已由 useRequest 统一提示
  editVisible.value = false
  applyEntries(list)
  ElMessage.success('已写回,建议重启对应工具使配置生效')
}

// 从接口添加模型弹窗:添加成功后用响应的最新清单整体刷新
const importVisible = ref(false)

/** 添加成功:应用响应中的最新清单(同时丢弃未保存的行内草稿);
 * 新建了供应商时通知父级刷新 Snapshot,保证落点候选 add_targets 及时更新。 */
function handleImported(result: AgentAddModelsResult, createdProvider: boolean) {
  applyEntries(result.entries)
  if (createdProvider) emit('providerCreated')
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
          <el-button v-if="canAddModels" type="primary" @click="importVisible = true">
            从接口添加模型
          </el-button>
          <el-button
            v-if="canRemoveModels"
            type="danger"
            :disabled="selectedRows.length === 0"
            :loading="removing"
            @click="handleRemoveSelected"
          >
            批量删除
          </el-button>
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

    <!-- 未找到配置:只展示指引,不渲染表格与增删改入口 -->
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
      <el-table
        v-loading="modelsLoading"
        :data="drafts"
        empty-text="该工具暂无模型条目"
        @selection-change="onSelectionChange"
      >
        <el-table-column v-if="canRemoveModels" type="selection" width="42" />
        <el-table-column
          v-for="col in snapshot.columns"
          :key="col.key"
          :label="col.label"
          :min-width="colWidth(col)"
        >
          <template #default="{ $index }">
            <AgentFieldInput
              :col="col"
              :model-value="cellValue($index, col.key)"
              @update:model-value="(v) => setCell($index, col.key, v)"
            />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="110" fixed="right">
          <template #default="{ $index }">
            <el-button link type="primary" @click="openEditDialog($index)">编辑</el-button>
            <el-button v-if="canRemoveModels" link type="danger" @click="handleRemoveRow($index)">
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="hasChanges" class="dirty-hint">
        有 {{ changedPatches.length }} 行修改未保存;保存前会自动备份原配置
      </div>
    </template>

    <!-- 编辑弹窗:按可编辑列渲染表单,确定即提交单行 patch -->
    <el-dialog v-model="editVisible" title="编辑模型" width="560px">
      <el-form label-width="110px">
        <el-form-item v-for="col in editableColumns" :key="col.key" :label="col.label">
          <AgentFieldInput
            :col="col"
            :model-value="editForm[col.key]"
            @update:model-value="(v) => setEditField(col.key, v)"
          />
          <div v-if="col.help" class="field-help">{{ col.help }}</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editVisible = false">取消</el-button>
        <el-button type="primary" :disabled="!hasEditChanges" :loading="saving" @click="handleEditSubmit">
          保存
        </el-button>
      </template>
    </el-dialog>

    <!-- 从接口添加模型:能力位不满足时不渲染 -->
    <AgentImportModelsDialog
      v-if="canAddModels"
      v-model:visible="importVisible"
      :snapshot="snapshot"
      @imported="handleImported"
    />
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

.field-help {
  width: 100%;
  font-size: 12px;
  line-height: 1.5;
  color: var(--el-text-color-secondary);
}

.dirty-hint {
  margin-top: 12px;
  font-size: 13px;
  color: var(--el-color-warning);
}
</style>
