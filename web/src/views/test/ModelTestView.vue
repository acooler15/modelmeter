<script setup lang="ts">
// 模型测试页:选择接口配置 + 模型(可手输,也可从配置拉取),按三种协议
// 发起对话实测(可开流式逐字渲染),展示回复、首字延迟、总耗时与 token
// 用量;下方保留最近测试记录(含失败)供回看,支持一键清空。
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'

import { fetchModels } from '@/api/modelCatalog'
import { estimateCost } from '@/api/newapi'
import { clearRecords, fetchRecords, runTest } from '@/api/modelTest'
import { useIsMobile } from '@/composables/useIsMobile'
import { useRequest } from '@/composables/useRequest'
import { isAbortError, useTestStream } from '@/composables/useTestStream'
import { useProvidersStore } from '@/stores/providers'
import type { CostEstimate } from '@/types/newapi'
import type { TestProtocol, TestRecord, TestResult } from '@/types/test'

// 窄屏判定:表单标签置顶、结果 descriptions 降为 1 列、详情弹窗全屏
const isMobile = useIsMobile()

const router = useRouter()
const providersStore = useProvidersStore()
const { list: providers } = storeToRefs(providersStore)

// 协议单选项:标签面向用户,值与后端协议标识一致
const protocolOptions: { label: string; value: TestProtocol }[] = [
  { label: 'OpenAI Chat Completions', value: 'chat_completions' },
  { label: 'OpenAI Responses', value: 'responses' },
  { label: 'Anthropic Messages', value: 'anthropic' },
]

function protocolLabel(p: TestProtocol): string {
  return protocolOptions.find((o) => o.value === p)?.label ?? p
}

// 表单:temperature/maxTokens 为 null 表示留空,不随请求发送
const form = reactive({
  providerId: null as number | null,
  model: '',
  protocol: 'chat_completions' as TestProtocol,
  system: '',
  user: '',
  stream: true,
})
const temperature = ref<number | null>(null)
const maxTokens = ref<number | null>(null)

// el-input-number 清空时给出 undefined,这里与「留空=null」语义双向转换
const temperatureInput = computed<number | undefined>({
  get: () => temperature.value ?? undefined,
  set: (v) => {
    temperature.value = v ?? null
  },
})
const maxTokensInput = computed<number | undefined>({
  get: () => maxTokens.value ?? undefined,
  set: (v) => {
    maxTokens.value = v ?? null
  },
})

// 切换配置后清空上一份模型下拉,避免误选其他配置的模型
watch(
  () => form.providerId,
  () => {
    modelOptions.value = []
  },
)

// 从所选配置拉取模型列表填充下拉(模型 ID 始终可手输)
const modelOptions = ref<string[]>([])
const { loading: modelsLoading, run: runFetchModels } = useRequest(fetchModels)

async function handleFetchModels() {
  if (form.providerId === null) return
  const models = await runFetchModels(form.providerId)
  if (models) modelOptions.value = models.map((m) => m.id)
}

// 测试执行:流式走 useTestStream 逐字渲染,非流式走 runTest 统一封装
const { streaming, start: startStream, abort: abortStream } = useTestStream()
const { loading: runLoading, run: runTestRequest } = useRequest(runTest)

const testing = computed(() => streaming.value || runLoading.value)
const streamingReply = ref('')
const result = ref<TestResult | null>(null)

// 流式过程中展示增量,结束后展示最终回复(两者内容一致)
const currentReply = computed(() => result.value?.reply ?? streamingReply.value)

// 成本估算:测试成功且有用量时按 New API 费率估算单次成本。
// 估算只是结果区的增强展示,任何失败都静默降级(后端亦以 available=false
// 返回 code=0),因此不走 useRequest 的统一错误提示,避免测试页弹无关报错。
const costEstimate = ref<CostEstimate | null>(null)
let estimateSeq = 0

/** 拉取单次成本估算;未配置/未命中/失败时保持 null,结果区不显示成本行。 */
async function refreshEstimate(model: string, finished: TestResult) {
  const seq = ++estimateSeq
  costEstimate.value = null
  const { prompt, completion } = finished.usage
  if (prompt <= 0 && completion <= 0) return // 上游未返回用量,无从估算
  try {
    const data = await estimateCost(model, prompt, completion)
    if (seq !== estimateSeq) return // 期间已发起新测试,丢弃过期结果
    if (data.available && data.estimate) costEstimate.value = data.estimate
  } catch {
    // 估算请求本身失败(如后端不可达)同样按「无成本行」处理,不打扰用户
  }
}

/** 发送测试;结束后刷新记录列表(失败也落库,同样需要刷新)。 */
async function handleSend() {
  if (form.providerId === null) {
    ElMessage.warning('请先选择接口配置')
    return
  }
  if (!form.model.trim()) {
    ElMessage.warning('请填写或选择模型')
    return
  }
  if (!form.user.trim()) {
    ElMessage.warning('请填写用户消息')
    return
  }

  result.value = null
  streamingReply.value = ''
  costEstimate.value = null
  try {
    if (form.stream) {
      const outcome = await startStream(
        {
          provider_id: form.providerId,
          protocol: form.protocol,
          model: form.model.trim(),
          system: form.system,
          user: form.user,
          temperature: temperature.value,
          max_tokens: maxTokens.value,
          stream: true,
        },
        (text) => {
          streamingReply.value += text
        },
      )
      result.value = outcome.result
    } else {
      const data = await runTestRequest({
        provider_id: form.providerId,
        protocol: form.protocol,
        model: form.model.trim(),
        system: form.system,
        user: form.user,
        temperature: temperature.value,
        max_tokens: maxTokens.value,
        stream: false,
      })
      // 失败时 useRequest 已弹中文提示,这里只接成功分支
      if (data) result.value = data.result
    }
    // 测试成功后尝试成本估算;失败静默,不影响测试结果展示
    if (result.value) void refreshEstimate(form.model.trim(), result.value)
  } catch (e) {
    if (isAbortError(e)) {
      ElMessage.info('已停止本次测试')
    } else {
      // 流式分支的错误不经过 useRequest,在此统一提示后端中文文案
      ElMessage.error(e instanceof Error ? e.message : '测试失败,请稍后重试')
    }
  } finally {
    void loadRecords()
  }
}

// 测试记录:页面私有数据,useRequest 持有
const { data: records, loading: recordsLoading, run: loadRecords } = useRequest(fetchRecords)
const { run: runClearRecords } = useRequest(clearRecords)

async function handleClearRecords() {
  const confirmed = await ElMessageBox.confirm('确定清空全部测试记录?该操作不可恢复', '清空记录', {
    type: 'warning',
    confirmButtonText: '清空',
    cancelButtonText: '取消',
  }).then(
    () => true,
    () => false,
  )
  if (!confirmed) return
  const res = await runClearRecords()
  if (res) ElMessage.success(`已清空 ${res.deleted} 条记录`)
  void loadRecords()
}

// 行点击查看完整回复与参数
const detailVisible = ref(false)
const detailRecord = ref<TestRecord | null>(null)

function showDetail(row: TestRecord) {
  detailRecord.value = row
  detailVisible.value = true
}

function formatTime(iso: string): string {
  return new Date(iso).toLocaleString('zh-CN', { hour12: false })
}

/** 用量三元组展示;三项全为 0 视为上游未返回。 */
function usageText(prompt: number, completion: number, total: number): string {
  if (prompt === 0 && completion === 0 && total === 0) return '未返回'
  return `${prompt} / ${completion} / ${total}`
}

// 离开页面时中断仍在进行的流式请求,避免悬挂连接
onBeforeUnmount(() => {
  abortStream()
})

onMounted(() => {
  void providersStore.load()
  void loadRecords()
})
</script>

<template>
  <div class="page">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>发起测试</span>
        </div>
      </template>

      <!-- 尚无任何接口配置:引导去配置页 -->
      <el-empty
        v-if="!providersStore.loading && providers.length === 0"
        description="还没有接口配置,先添加一个才能发起测试"
      >
        <el-button type="primary" @click="router.push('/providers')">去接口配置页</el-button>
      </el-empty>

      <template v-else>
        <el-form
          label-width="130px"
          :label-position="isMobile ? 'top' : 'right'"
          :disabled="testing"
          class="test-form"
        >
          <el-form-item label="接口配置">
            <el-select
              v-model="form.providerId"
              class="provider-select"
              placeholder="选择接口配置"
            >
              <el-option v-for="p in providers" :key="p.id" :label="p.name" :value="p.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="模型">
            <div class="model-row">
              <el-select
                v-model="form.model"
                class="model-select"
                filterable
                allow-create
                default-first-option
                placeholder="输入模型 ID,或点击右侧「从配置拉取」后选择"
              >
                <el-option v-for="m in modelOptions" :key="m" :label="m" :value="m" />
              </el-select>
              <el-button
                :disabled="form.providerId === null"
                :loading="modelsLoading"
                @click="handleFetchModels"
              >
                从配置拉取
              </el-button>
            </div>
          </el-form-item>
          <el-form-item label="协议">
            <el-radio-group v-model="form.protocol">
              <el-radio-button v-for="o in protocolOptions" :key="o.value" :value="o.value">
                {{ o.label }}
              </el-radio-button>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="System 提示词">
            <el-input
              v-model="form.system"
              type="textarea"
              :rows="2"
              placeholder="可选;留空则不发送 system"
            />
          </el-form-item>
          <el-form-item label="用户消息">
            <el-input
              v-model="form.user"
              type="textarea"
              :rows="3"
              placeholder="必填;即发送给模型的提问内容"
            />
          </el-form-item>
          <el-form-item label="temperature">
            <el-input-number
              v-model="temperatureInput"
              :min="0"
              :step="0.1"
              :value-on-clear="null"
              placeholder="留空用默认"
            />
          </el-form-item>
          <el-form-item label="max_tokens">
            <el-input-number
              v-model="maxTokensInput"
              :min="1"
              :step="1"
              :value-on-clear="null"
              placeholder="留空用默认"
            />
            <span v-if="form.protocol === 'anthropic'" class="hint">
              Anthropic 协议必填,留空时后端自动按 1024 发送
            </span>
          </el-form-item>
          <el-form-item label="流式输出">
            <el-switch v-model="form.stream" />
            <span class="hint">开启后逐字渲染回复</span>
          </el-form-item>
        </el-form>

        <!-- 动作按钮放在表单外:发送中需保持「停止」可用 -->
        <div class="actions">
          <el-button type="primary" :loading="testing" :disabled="testing" @click="handleSend">
            发送测试
          </el-button>
          <el-button v-if="streaming" type="warning" plain @click="abortStream">停止</el-button>
        </div>
      </template>
    </el-card>

    <el-card class="section">
      <template #header>
        <div class="card-header">
          <span>测试结果</span>
          <el-tag v-if="streaming" type="warning" size="small">生成中…</el-tag>
        </div>
      </template>
      <el-empty
        v-if="!currentReply && !streaming"
        description="发送测试后在这里查看回复与指标"
        :image-size="80"
      />
      <template v-else>
        <pre class="reply">{{ currentReply }}</pre>
        <el-descriptions v-if="result" :column="isMobile ? 1 : 3" border size="small" class="metrics">
          <el-descriptions-item label="首字延迟">
            {{ result.first_latency_ms }} ms
          </el-descriptions-item>
          <el-descriptions-item label="总耗时">
            {{ result.total_latency_ms }} ms
          </el-descriptions-item>
          <el-descriptions-item label="Tokens(输入/输出/总计)">
            {{ usageText(result.usage.prompt, result.usage.completion, result.usage.total) }}
          </el-descriptions-item>
          <!-- 成本行只在费率命中时出现;未配置/未命中/失败都不显示、不报错 -->
          <el-descriptions-item v-if="costEstimate" label="估算成本" :span="3">
            <span class="cost-value">约 ${{ costEstimate.usd.toFixed(6) }}</span>
            <span class="cost-formula">{{ costEstimate.formula }}</span>
          </el-descriptions-item>
        </el-descriptions>
      </template>
    </el-card>

    <el-card class="section">
      <template #header>
        <div class="card-header">
          <span>测试记录(最近 200 条)</span>
          <div>
            <el-button size="small" :loading="recordsLoading" @click="loadRecords()">
              刷新
            </el-button>
            <el-button
              size="small"
              type="danger"
              plain
              :disabled="(records?.length ?? 0) === 0"
              @click="handleClearRecords"
            >
              清空记录
            </el-button>
          </div>
        </div>
      </template>
      <el-empty
        v-if="(records?.length ?? 0) === 0 && !recordsLoading"
        description="暂无测试记录"
        :image-size="80"
      />
      <el-table
        v-else
        v-loading="recordsLoading"
        :data="records ?? []"
        highlight-current-row
        @row-click="showDetail"
      >
        <el-table-column label="时间" min-width="165">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column prop="provider_name" label="配置" min-width="110" show-overflow-tooltip />
        <el-table-column prop="model" label="模型" min-width="170" show-overflow-tooltip />
        <el-table-column label="协议" min-width="150" show-overflow-tooltip>
          <template #default="{ row }">{{ protocolLabel(row.protocol) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="76">
          <template #default="{ row }">
            <el-tag :type="row.succeeded ? 'success' : 'danger'" size="small">
              {{ row.succeeded ? '成功' : '失败' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="耗时" width="90">
          <template #default="{ row }">{{ row.total_latency_ms }} ms</template>
        </el-table-column>
        <el-table-column label="Tokens" width="90">
          <template #default="{ row }">{{ row.total_tokens || '未返回' }}</template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 记录详情:完整回复与当时的参数快照;窄屏全屏化并将描述列表降为 1 列 -->
    <el-dialog v-model="detailVisible" title="测试详情" width="680px" :fullscreen="isMobile">
      <el-descriptions v-if="detailRecord" :column="isMobile ? 1 : 2" border size="small">
        <el-descriptions-item label="时间">
          {{ formatTime(detailRecord.created_at) }}
        </el-descriptions-item>
        <el-descriptions-item label="状态">
          {{ detailRecord.succeeded ? '成功' : '失败' }}
        </el-descriptions-item>
        <el-descriptions-item label="配置">{{ detailRecord.provider_name }}</el-descriptions-item>
        <el-descriptions-item label="模型">{{ detailRecord.model }}</el-descriptions-item>
        <el-descriptions-item label="协议">
          {{ protocolLabel(detailRecord.protocol) }}
        </el-descriptions-item>
        <el-descriptions-item label="方式">
          {{ detailRecord.stream ? '流式' : '非流式' }}
        </el-descriptions-item>
        <el-descriptions-item label="temperature">
          {{ detailRecord.temperature ?? '未设置' }}
        </el-descriptions-item>
        <el-descriptions-item label="max_tokens">
          {{ detailRecord.max_tokens ?? '未设置' }}
        </el-descriptions-item>
        <el-descriptions-item label="首字延迟">
          {{ detailRecord.first_latency_ms }} ms
        </el-descriptions-item>
        <el-descriptions-item label="总耗时">
          {{ detailRecord.total_latency_ms }} ms
        </el-descriptions-item>
        <el-descriptions-item label="Tokens(输入/输出/总计)" :span="2">
          {{
            usageText(
              detailRecord.prompt_tokens,
              detailRecord.completion_tokens,
              detailRecord.total_tokens,
            )
          }}
        </el-descriptions-item>
        <el-descriptions-item v-if="detailRecord.system_prompt" label="System 提示词" :span="2">
          {{ detailRecord.system_prompt }}
        </el-descriptions-item>
        <el-descriptions-item label="用户消息" :span="2">
          {{ detailRecord.user_message }}
        </el-descriptions-item>
        <el-descriptions-item
          v-if="detailRecord.succeeded"
          label="完整回复"
          :span="2"
        >
          <pre class="detail-reply">{{ detailRecord.reply }}</pre>
        </el-descriptions-item>
        <el-descriptions-item v-else label="失败原因" :span="2">
          {{ detailRecord.err_message }}
        </el-descriptions-item>
      </el-descriptions>
    </el-dialog>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.test-form {
  max-width: 720px;
}

.provider-select {
  width: 260px;
}

/* 窄屏:接口配置下拉占满可用行宽(模型行 .model-row 本身弹性布局,无需处理) */
@media (max-width: 768px) {
  .provider-select {
    width: 100%;
  }
}

.model-row {
  display: flex;
  gap: 12px;
  width: 100%;
}

.model-select {
  flex: 1;
}

.hint {
  margin-left: 12px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.actions {
  display: flex;
  gap: 12px;
}

.reply {
  margin: 0 0 16px;
  padding: 12px 16px;
  max-height: 420px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 14px;
  line-height: 1.7;
  color: var(--el-text-color-primary);
  background: var(--el-fill-color-light);
  border-radius: 4px;
}

.metrics {
  margin-bottom: 12px;
}

.cost-value {
  font-weight: 600;
  color: var(--el-color-primary);
}

.cost-formula {
  margin-left: 12px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.detail-reply {
  margin: 0;
  max-height: 320px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 13px;
  line-height: 1.6;
}
</style>
