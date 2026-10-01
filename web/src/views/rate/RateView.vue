<script setup lang="ts">
// 模型费率页:维护 New API 网关配置(地址 + 访问令牌),实时拉取模型费率表
// 并支持按模型名关键字过滤;未配置时引导先保存配置。令牌一律脱敏展示,
// 编辑留空表示沿用原令牌;错误提示由 useRequest 统一弹中文消息。
import { computed, onMounted, reactive, ref } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'

import {
  fetchNewAPIRates,
  getNewAPIConfig,
  saveNewAPIConfig,
} from '@/api/newapi'
import { useIsMobile } from '@/composables/useIsMobile'
import { useRequest } from '@/composables/useRequest'
import type { NewAPIConfigView } from '@/types/newapi'

// 窄屏判定:配置表单标签置顶
const isMobile = useIsMobile()

// 配置:进入页面拉取一次;保存成功后刷新视图并尝试拉取费率表
const config = ref<NewAPIConfigView | null>(null)
const configFormRef = ref<FormInstance>()
const configForm = reactive({ base_url: '', token: '' })

const { run: runGetConfig, loading: configLoading } = useRequest(getNewAPIConfig)
const { run: runSaveConfig, loading: saving } = useRequest(saveNewAPIConfig)
// ratesError 供持久错误提示展示;拉取失败时不能误显「上游没有条目」空态
const {
  data: rates,
  loading: ratesLoading,
  error: ratesError,
  run: loadRates,
} = useRequest(fetchNewAPIRates)

// 令牌可留空(沿用原令牌),校验交给后端,这里只校验地址
const rules: FormRules = {
  base_url: [
    { required: true, message: '请输入 New API 地址', trigger: 'blur' },
    { pattern: /^https?:\/\/\S+$/i, message: 'New API 地址必须是 http(s) 地址', trigger: 'blur' },
  ],
}

/** 拉取配置视图并回填表单;令牌不回填,留空提交即沿用原令牌。 */
async function loadConfig() {
  const view = await runGetConfig()
  if (!view) return
  config.value = view
  configForm.base_url = view.base_url
  configForm.token = ''
}

/** 保存配置:成功后刷新视图与费率表。 */
async function handleSaveConfig() {
  if (!configFormRef.value) return
  const valid = await configFormRef.value.validate().catch(() => false)
  if (!valid) return
  const view = await runSaveConfig({ ...configForm })
  if (view === undefined) return // 失败已由 useRequest 统一提示
  config.value = view
  configForm.base_url = view.base_url
  configForm.token = ''
  ElMessage.success('保存成功')
  void loadRates()
}

// 关键字过滤:按模型名模糊匹配,大小写不敏感
const keyword = ref('')
const filteredRates = computed(() => {
  const list = rates.value ?? []
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return list
  return list.filter((r) => r.model_name.toLowerCase().includes(kw))
})

// 表格空态文案:有关键字时提示过滤无结果,否则提示上游没有返回条目
// (与模型列表页一致;项目未配置 Element Plus 中文 locale,默认空态是英文)
const emptyText = computed(() =>
  keyword.value.trim() ? '没有匹配的模型' : '上游未返回任何费率条目',
)

/** 计费类型中文标签;上游未知类型按「未知」展示。 */
function quotaTypeLabel(quotaType: number): string {
  if (quotaType === 0) return '倍率'
  if (quotaType === 1) return '按次'
  return '未知'
}

/** 倍率列只在倍率计费行显示数值,按次行显示「—」。 */
function ratioCell(quotaType: number, value: number): string {
  return quotaType === 0 ? String(value) : '—'
}

/** 价格列只在按次计费行显示数值,倍率行显示「—」。 */
function priceCell(quotaType: number, value: number): string {
  return quotaType === 1 ? `$${value}` : '—'
}

onMounted(async () => {
  await loadConfig()
  // 已配置过的用户进入页面直接带出费率表
  if (config.value?.configured) void loadRates()
})
</script>

<template>
  <div class="page">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>New API 配置</span>
          <span v-if="config?.configured" class="configured-hint">
            已配置:{{ config.base_url }}(令牌 {{ config.token_masked }})
          </span>
        </div>
      </template>

      <el-form
        ref="configFormRef"
        v-loading="configLoading"
        :model="configForm"
        :rules="rules"
        label-width="110px"
        :label-position="isMobile ? 'top' : 'right'"
        class="config-form"
      >
        <el-form-item label="网关地址" prop="base_url">
          <el-input
            v-model="configForm.base_url"
            placeholder="例如:https://newapi.example.com"
            maxlength="500"
          />
        </el-form-item>
        <el-form-item label="访问令牌" prop="token">
          <el-input
            v-model="configForm.token"
            type="password"
            show-password
            autocomplete="new-password"
            placeholder="留空则沿用原令牌"
          />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" @click="handleSaveConfig">保存</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card class="section">
      <template #header>
        <div class="card-header">
          <span>模型费率</span>
          <div class="toolbar">
            <el-input
              v-model="keyword"
              class="filter-input"
              placeholder="按模型名过滤"
              clearable
            />
            <el-button :loading="ratesLoading" @click="loadRates()">刷新</el-button>
          </div>
        </div>
      </template>

      <!-- 配置未就绪时不误显引导;未配置时引导去上方配置卡片 -->
      <el-empty
        v-if="!configLoading && !config?.configured"
        description="请先在上方保存 New API 地址与访问令牌,再拉取模型费率"
        :image-size="80"
      />
      <template v-else>
        <!-- 拉取失败时持久展示后端中文文案,与模型列表页一致 -->
        <el-alert
          v-if="ratesError"
          class="rates-error"
          type="error"
          :title="ratesError.message"
          show-icon
          :closable="false"
        />
        <el-empty
          v-else-if="rates === undefined"
          v-loading="ratesLoading"
          description="暂无费率数据,点击「刷新」获取"
          :image-size="80"
        />
        <el-table
          v-else
          v-loading="ratesLoading"
          :data="filteredRates"
          :empty-text="emptyText"
          highlight-current-row
        >
          <el-table-column prop="model_name" label="模型" min-width="220" show-overflow-tooltip />
          <el-table-column label="计费类型" width="96">
            <template #default="{ row }">
              <el-tag :type="row.quota_type === 1 ? 'warning' : 'primary'" size="small">
                {{ quotaTypeLabel(row.quota_type) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="输入倍率" width="120">
            <template #default="{ row }">{{ ratioCell(row.quota_type, row.model_ratio) }}</template>
          </el-table-column>
          <el-table-column label="完成倍率" width="120">
            <template #default="{ row }">
              {{ ratioCell(row.quota_type, row.completion_ratio) }}
            </template>
          </el-table-column>
          <el-table-column label="价格(美元/次)" min-width="130">
            <template #default="{ row }">{{ priceCell(row.quota_type, row.model_price) }}</template>
          </el-table-column>
        </el-table>
      </template>
    </el-card>
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

.configured-hint {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  /* 长 URL 提示允许折行,不撑爆容器(对桌面端无副作用) */
  word-break: break-all;
}

.config-form {
  max-width: 560px;
}

.toolbar {
  display: flex;
  gap: 12px;
}

.filter-input {
  width: 220px;
}

/* 窄屏:过滤输入框占满可用行宽,与刷新按钮随全局换行规则堆叠 */
@media (max-width: 768px) {
  .filter-input {
    width: 100%;
  }
}

.rates-error {
  margin-bottom: 16px;
}
</style>
