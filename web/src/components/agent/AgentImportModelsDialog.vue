<script setup lang="ts">
// 从接口添加模型弹窗:选择接口 → 拉取模型列表 → 多选/全选 → 确认接口地址
// (WorkBuddy/CodeBuddy 提示后端将派生的完整 endpoint)→ ZCode 选择落点
// (挂已有供应商或新建供应商)→ 二次确认(涉凭据写入的路径明确警示)→ 提交。
// 凭据不经过前端:请求体只含接口 ID,后端从接口记录装配,响应不含 key。
// 接口列表来自共享 providers store;派生 endpoint 的提示与后端
// llmclient.BuildURL 口径一致,实际派生在后端。
import { computed, ref, watch } from 'vue'

import { addAgentModels } from '@/api/agent'
import { fetchModels } from '@/api/modelCatalog'
import { useRequest } from '@/composables/useRequest'
import { useProvidersStore } from '@/stores/providers'
import type { AgentAddModelsPayload, AgentAddModelsResult, AgentSnapshot } from '@/types/agent'
import type { ModelInfo } from '@/types/model'

const props = defineProps<{ snapshot: AgentSnapshot; visible: boolean }>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  /** 第二参数=是否新建了供应商:父级需据此刷新 Snapshot(add_targets 已变化)。 */
  imported: [result: AgentAddModelsResult, createdProvider: boolean]
}>()

const providersStore = useProvidersStore()
const { run: runFetchModels, loading: modelsLoading, data: models } = useRequest(fetchModels)
const { run: runAdd, loading: adding } = useRequest(addAgentModels)

// 落点下拉的"新建供应商"特殊项取值;与真实 provider_id 不会冲突
const NEW_TARGET = '__new__'

const providerId = ref<number | null>(null)
const baseUrl = ref('')
const selectedRows = ref<ModelInfo[]>([])
const targetKey = ref(NEW_TARGET)
const newProviderName = ref('')
const apiType = ref<'openai-chat-completions' | 'openai-responses'>('openai-chat-completions')

// ZCode 判别:落点选择是 ZCode 特有交互;add_targets 为空时被后端
// omitempty 省略、无法用字段区分,故用稳定的工具标识 name 判断
const isZcode = computed(() => props.snapshot.name === 'zcode')
const isTargetNew = computed(() => targetKey.value === NEW_TARGET)

const selectedProvider = computed(() =>
  providersStore.list.find((item) => item.id === providerId.value),
)

/** 弹窗打开时重置流程状态,并确保接口列表就绪(store 已加载过则跳过)。 */
watch(
  () => props.visible,
  (visible) => {
    if (!visible) return
    providerId.value = null
    baseUrl.value = ''
    selectedRows.value = []
    models.value = undefined
    targetKey.value = props.snapshot.add_targets?.[0]?.provider_id ?? NEW_TARGET
    newProviderName.value = ''
    apiType.value = 'openai-chat-completions'
    void providersStore.load()
  },
)

/** 选择接口:预填接口地址与新建供应商显示名,清空多选并拉取模型列表。 */
function onProviderChange(id: number) {
  const provider = providersStore.list.find((item) => item.id === id)
  baseUrl.value = provider?.base_url ?? ''
  newProviderName.value = provider?.name ?? ''
  selectedRows.value = []
  models.value = undefined
  void runFetchModels(id)
}

/** 模型按清单顺序取已选 id,避免按勾选顺序提交导致输入顺序不稳定。 */
const orderedSelectedIds = computed(() => {
  const chosen = new Set(selectedRows.value.map((row) => row.id))
  return (models.value ?? []).filter((item) => chosen.has(item.id)).map((item) => item.id)
})

/** 实际生效的接口地址:输入为空时回退接口记录的 Base URL(后端同口径)。 */
const effectiveBase = computed(() => baseUrl.value.trim() || selectedProvider.value?.base_url || '')

/**
 * 镜像后端 llmclient.BuildURL 的派生口径:去尾斜杠,base 已以 /v1 结尾时
 * 不再重复拼接 path 的 /v1 前缀。仅用于前端提示文案,实际派生在后端。
 */
const derivedEndpoint = computed(() => {
  const base = effectiveBase.value.replace(/\/+$/, '')
  const path = base.endsWith('/v1') ? '/chat/completions' : '/v1/chat/completions'
  return base + path
})

/** 提交可用性:已选接口与模型;ZCode 挂靠模式必须已选落点。 */
const canSubmit = computed(() => {
  if (providerId.value === null) return false
  if (orderedSelectedIds.value.length === 0) return false
  if (isZcode.value && !isTargetNew.value && targetKey.value === '') return false
  return true
})

/** 提交添加:按工具与落点模式给出对应的二次确认文案后请求。 */
async function handleConfirm() {
  if (!canSubmit.value || providerId.value === null) return
  const count = orderedSelectedIds.value.length
  const providerName = selectedProvider.value?.name ?? ''
  let message: string
  if (!isZcode.value) {
    message = `将把所选接口「${providerName}」的 API Key 写入「${props.snapshot.display_name}」配置文件,确定继续吗?`
  } else if (isTargetNew.value) {
    message = `将在「${props.snapshot.display_name}」配置文件中新建供应商并写入所选接口「${providerName}」的 API Key,确定继续吗?`
  } else {
    const target = (props.snapshot.add_targets ?? []).find(
      (item) => item.provider_id === targetKey.value,
    )
    message = `确定向「${props.snapshot.display_name}」的供应商「${target?.provider_name || targetKey.value}」添加 ${count} 个模型吗?`
  }
  try {
    await ElMessageBox.confirm(message, '添加确认', {
      type: 'warning',
      confirmButtonText: '确认添加',
      cancelButtonText: '取消',
    })
  } catch {
    return // 用户取消,无需提示
  }
  const payload: AgentAddModelsPayload = {
    provider_id: providerId.value,
    model_ids: orderedSelectedIds.value,
    base_url: baseUrl.value.trim(),
  }
  if (isZcode.value) {
    payload.target = isTargetNew.value
      ? { mode: 'new', provider_name: newProviderName.value.trim(), api_type: apiType.value }
      : { mode: 'existing', provider_id: targetKey.value }
  }
  const result = await runAdd(props.snapshot.name, payload)
  if (result === undefined) return // 失败已由 useRequest 统一提示
  const skippedNote =
    result.skipped.length > 0 ? `,跳过 ${result.skipped.length} 个(已存在)` : ''
  ElMessage.success(`成功添加 ${result.added.length} 个模型${skippedNote}`)
  emit('imported', result, isZcode.value && isTargetNew.value && result.added.length > 0)
  emit('update:visible', false)
}
</script>

<template>
  <el-dialog
    :model-value="visible"
    :title="`从接口添加模型到 ${snapshot.display_name}`"
    width="640px"
    @update:model-value="emit('update:visible', $event)"
  >
    <el-form label-width="90px">
      <el-form-item label="选择接口" required>
        <el-select
          v-model="providerId"
          class="provider-select"
          placeholder="请选择接口"
          filterable
          @change="onProviderChange"
        >
          <el-option
            v-for="p in providersStore.list"
            :key="p.id"
            :label="p.name"
            :value="p.id"
          >
            <span>{{ p.name }}</span>
            <span class="provider-url">{{ p.base_url }}</span>
          </el-option>
        </el-select>
      </el-form-item>
    </el-form>

    <el-empty
      v-if="providersStore.list.length === 0 && !providersStore.loading"
      description="暂无接口配置,请先在「接口配置」页新增"
    />
    <template v-else-if="providerId !== null">
      <el-table
        v-loading="modelsLoading"
        :data="models ?? []"
        max-height="280"
        empty-text="该接口未返回模型"
        @selection-change="selectedRows = $event"
      >
        <el-table-column type="selection" width="42" />
        <el-table-column prop="id" label="模型 ID" min-width="240" show-overflow-tooltip />
        <el-table-column prop="owned_by" label="归属" min-width="140" show-overflow-tooltip />
      </el-table>
      <div class="selection-hint">已选 {{ orderedSelectedIds.length }} 个模型(表头可全选)</div>
    </template>

    <el-form label-width="90px" class="option-form">
      <el-form-item label="接口地址">
        <el-input v-model="baseUrl" placeholder="留空则使用接口配置的 Base URL" />
        <!-- WorkBuddy/CodeBuddy 的 url 是完整 endpoint 语义;ZCode 忽略该概念 -->
        <div v-if="!isZcode && effectiveBase" class="derive-hint">
          实际写入条目的完整地址:{{ derivedEndpoint }}
        </div>
      </el-form-item>
      <template v-if="isZcode">
        <el-form-item label="添加到" required>
          <el-select v-model="targetKey" placeholder="请选择落点">
            <el-option
              v-for="target in snapshot.add_targets ?? []"
              :key="target.provider_id"
              :label="target.provider_name || target.provider_id"
              :value="target.provider_id"
            />
            <el-option label="新建供应商…" :value="NEW_TARGET" />
          </el-select>
        </el-form-item>
        <template v-if="isTargetNew">
          <el-form-item label="显示名">
            <el-input
              v-model="newProviderName"
              placeholder="新建供应商的显示名,留空使用接口名"
              maxlength="100"
            />
          </el-form-item>
          <el-form-item label="API 协议">
            <el-select v-model="apiType">
              <el-option label="OpenAI Chat Completions" value="openai-chat-completions" />
              <el-option label="OpenAI Responses" value="openai-responses" />
            </el-select>
          </el-form-item>
        </template>
      </template>
    </el-form>

    <template #footer>
      <el-button @click="emit('update:visible', false)">取消</el-button>
      <el-button type="primary" :disabled="!canSubmit" :loading="adding" @click="handleConfirm">
        添加
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.provider-select {
  width: 100%;
}

.provider-url {
  float: right;
  margin-left: 12px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.selection-hint {
  margin-top: 8px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.option-form {
  margin-top: 12px;
}

.derive-hint {
  width: 100%;
  font-size: 12px;
  line-height: 1.5;
  color: var(--el-text-color-secondary);
  word-break: break-all;
}
</style>
