<script setup lang="ts">
// 模型列表页:选择接口配置后经后端代理拉取上游 /v1/models 并展示。
// 拉取失败时 el-alert 持久展示后端中文文案(网络/鉴权错误可区分),
// 同时由 useRequest 统一弹出提示;点击行或展开箭头可查看上游原始元信息。
import { computed, onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'
import type { TableInstance } from 'element-plus'

import { fetchModels } from '@/api/modelCatalog'
import { useRequest } from '@/composables/useRequest'
import { useProvidersStore } from '@/stores/providers'
import type { ModelInfo } from '@/types/model'

const router = useRouter()
const providersStore = useProvidersStore()
const { list: providers, loading: providersLoading } = storeToRefs(providersStore)

// 当前选中的配置 ID;null 表示未选择
const selectedId = ref<number | null>(null)

// 模型拉取是页面私有请求,走 useRequest;error 供 el-alert 展示后端文案
const {
  data: models,
  loading: modelsLoading,
  error: modelsError,
  run: runFetchModels,
} = useRequest(fetchModels)

const tableRef = ref<TableInstance>()

// 关键字过滤:大小写不敏感地匹配模型 ID 或 owned_by
const keyword = ref('')
const filteredModels = computed(() => {
  const all = models.value ?? []
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return all
  return all.filter(
    (m) => m.id.toLowerCase().includes(kw) || m.owned_by.toLowerCase().includes(kw),
  )
})

// 表格空态文案:有关键字时提示过滤无结果,否则提示上游没有返回模型
const emptyText = computed(() => (keyword.value.trim() ? '没有匹配的模型' : '上游未返回任何模型'))

// 切换配置后清空上一份模型列表与错误提示,避免误读旧数据
watch(selectedId, () => {
  models.value = undefined
  modelsError.value = null
})

/** 拉取选中配置的模型列表;未选择配置时不发请求。 */
async function handleFetch() {
  if (selectedId.value === null) return
  await runFetchModels(selectedId.value)
}

/** 刷新:先同步一次配置列表(吸收其他页面的变更),再重拉模型列表。 */
async function handleRefresh() {
  if (selectedId.value === null) return
  void providersStore.load(true)
  await handleFetch()
}

/** 点击行展开/收起元信息;点击展开箭头时交给 Element Plus 默认行为,避免二次翻转。 */
function handleRowClick(row: ModelInfo, column: { type?: string } | null) {
  if (column?.type === 'expand') return
  tableRef.value?.toggleRowExpansion(row)
}

/** created 为 Unix 秒级时间戳,0 或缺失时显示占位符。 */
function formatCreated(seconds: number): string {
  if (!seconds) return '-'
  return new Date(seconds * 1000).toLocaleString('zh-CN', { hour12: false })
}

/** 上游原始条目结构不完全可信,统一走 JSON 序列化展示。 */
function formatRaw(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2) ?? ''
  } catch {
    return String(value)
  }
}

onMounted(() => {
  void providersStore.load()
})
</script>

<template>
  <el-card>
    <template #header>
      <div class="card-header">
        <span>模型列表</span>
      </div>
    </template>

    <!-- 尚无任何接口配置:引导去配置页 -->
    <el-empty
      v-if="!providersLoading && providers.length === 0"
      description="还没有接口配置,先添加一个才能拉取模型"
    >
      <el-button type="primary" @click="router.push('/providers')">去接口配置页</el-button>
    </el-empty>

    <template v-else>
      <div class="toolbar">
        <el-select v-model="selectedId" class="provider-select" placeholder="选择接口配置">
          <el-option v-for="p in providers" :key="p.id" :label="p.name" :value="p.id" />
        </el-select>
        <el-button
          type="primary"
          :disabled="selectedId === null"
          :loading="modelsLoading"
          @click="handleFetch"
        >
          拉取模型
        </el-button>
        <el-button :disabled="selectedId === null" :loading="modelsLoading" @click="handleRefresh">
          刷新
        </el-button>
        <el-input
          v-model="keyword"
          class="keyword-input"
          placeholder="按模型 ID / owned_by 过滤"
          clearable
        />
      </div>

      <!-- 错误提示直接使用后端返回的中文文案 -->
      <el-alert
        v-if="modelsError"
        class="error-alert"
        type="error"
        :title="modelsError.message"
        show-icon
        :closable="false"
      />

      <el-empty
        v-if="models === undefined"
        v-loading="modelsLoading"
        description="暂无模型数据,选择配置后点击「拉取模型」获取"
      />
      <el-table
        v-else
        ref="tableRef"
        v-loading="modelsLoading"
        :data="filteredModels"
        :empty-text="emptyText"
        @row-click="handleRowClick"
      >
        <el-table-column type="expand">
          <template #default="{ row }">
            <pre class="raw-json">{{ formatRaw(row.raw) }}</pre>
          </template>
        </el-table-column>
        <el-table-column prop="id" label="模型 ID" min-width="260" show-overflow-tooltip />
        <el-table-column prop="owned_by" label="owned_by" min-width="140" />
        <el-table-column label="创建时间" min-width="170">
          <template #default="{ row }">{{ formatCreated(row.created) }}</template>
        </el-table-column>
      </el-table>
    </template>
  </el-card>
</template>

<style scoped>
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.toolbar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
}

.provider-select {
  width: 240px;
}

.keyword-input {
  width: 240px;
  margin-left: auto;
}

/* 窄屏:下拉与搜索框占满可用行宽,控件逐行堆叠,按钮行随全局 flex-wrap 换行 */
@media (max-width: 768px) {
  .provider-select,
  .keyword-input {
    width: 100%;
  }

  .keyword-input {
    margin-left: 0;
  }
}

.error-alert {
  margin-bottom: 16px;
}

.raw-json {
  margin: 0;
  padding: 8px 16px;
  max-height: 320px;
  overflow: auto;
  font-size: 12px;
  line-height: 1.6;
  color: var(--el-text-color-regular);
  background: var(--el-fill-color-light);
  border-radius: 4px;
}
</style>
