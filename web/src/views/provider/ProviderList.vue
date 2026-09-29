<script setup lang="ts">
// 接口配置页:管理站点名称、Base URL 与 API Key 的增删改查。
// Key 一律脱敏展示;编辑时留空表示沿用原 Key;错误提示由 useRequest 统一处理。
// 列表数据源为 providers store(与模型列表页共享);写操作各自持有 loading,
// 失败时均由 useRequest 统一弹中文提示。
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { storeToRefs } from 'pinia'
import type { FormInstance, FormRules } from 'element-plus'

import {
  createProvider,
  deleteProvider,
  updateProvider,
} from '@/api/provider'
import { useRequest } from '@/composables/useRequest'
import { useProvidersStore } from '@/stores/providers'
import type { ProviderInput, ProviderView } from '@/types/provider'

// 列表数据来自共享 store;本页写操作成功后用 force 刷新,保证与其他页面同源
const providersStore = useProvidersStore()
const { list: providers, loading: listLoading } = storeToRefs(providersStore)
const { run: runCreate, loading: creating } = useRequest(createProvider)
const { run: runUpdate, loading: updating } = useRequest(updateProvider)
const { run: runDelete } = useRequest(deleteProvider)

// 对话框状态:editing 为 null 表示新增
const dialogVisible = ref(false)
const editing = ref<ProviderView | null>(null)
const formRef = ref<FormInstance>()
const form = reactive<ProviderInput>({ name: '', base_url: '', api_key: '' })

// 校验规则:Key 仅新增时必填,编辑留空表示沿用原值
const rules = computed<FormRules>(() => ({
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  base_url: [
    { required: true, message: '请输入 Base URL', trigger: 'blur' },
    { pattern: /^https?:\/\/\S+$/i, message: 'Base URL 必须是 http(s) 地址', trigger: 'blur' },
  ],
  api_key: editing.value
    ? []
    : [{ required: true, message: '请输入 API Key', trigger: 'blur' }],
}))

/** el-table 插槽的 row 由 Element Plus 以弱类型(DefaultRow)提供,
 * 属于不可信来源,按规范先运行时收窄再当作 ProviderView 使用。 */
function isProviderRow(x: unknown): x is ProviderView {
  return (
    typeof x === 'object' &&
    x !== null &&
    'id' in x &&
    'name' in x &&
    'base_url' in x &&
    'api_key_masked' in x &&
    'updated_at' in x
  )
}

/** 打开对话框并重置表单:row 为 null 表示新增。 */
function openDialog(row: unknown) {
  const target = isProviderRow(row) ? row : null
  editing.value = target
  Object.assign(form, {
    name: target?.name ?? '',
    base_url: target?.base_url ?? '',
    api_key: '',
  })
  dialogVisible.value = true
  // 清除上一次打开遗留的校验状态
  void nextTick(() => formRef.value?.clearValidate())
}

/** 提交表单:按编辑状态分别调用新增或编辑接口。 */
async function handleSubmit() {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  const input: ProviderInput = { ...form }
  const saved = editing.value
    ? await runUpdate(editing.value.id, input)
    : await runCreate(input)
  if (saved === undefined) return // 失败已由 useRequest 统一提示
  ElMessage.success(editing.value ? '保存成功' : '新增成功')
  dialogVisible.value = false
  void providersStore.load(true)
}

/** 删除前二次确认,确认后调用删除接口并刷新列表。 */
async function handleDelete(row: unknown) {
  if (!isProviderRow(row)) return
  try {
    await ElMessageBox.confirm(
      `确定删除接口配置「${row.name}」吗?删除后不可恢复。`,
      '删除确认',
      { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
  } catch {
    return // 用户取消,无需提示
  }
  const deleted = await runDelete(row.id)
  if (deleted === undefined) return
  ElMessage.success('删除成功')
  void providersStore.load(true)
}

/** 后端时间为 UTC ISO 字符串,转本地时间展示。 */
function formatTime(value: string): string {
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

onMounted(() => {
  void providersStore.load()
})
</script>

<template>
  <el-card>
    <template #header>
      <div class="card-header">
        <span>接口配置</span>
        <el-button type="primary" @click="openDialog(null)">新增配置</el-button>
      </div>
    </template>

    <el-empty
      v-if="!listLoading && providers.length === 0"
      description="还没有接口配置,先新增一个才能测试模型"
    >
      <el-button type="primary" @click="openDialog(null)">新增配置</el-button>
    </el-empty>

    <el-table v-else v-loading="listLoading" :data="providers">
      <el-table-column prop="name" label="名称" min-width="140" />
      <el-table-column prop="base_url" label="Base URL" min-width="220" show-overflow-tooltip />
      <el-table-column prop="api_key_masked" label="API Key" min-width="140" />
      <el-table-column label="更新时间" min-width="170">
        <template #default="{ row }">{{ formatTime(row.updated_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="140">
        <template #default="{ row }">
          <el-button link type="primary" @click="openDialog(row)">编辑</el-button>
          <el-button link type="danger" @click="handleDelete(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog
      v-model="dialogVisible"
      :title="editing ? '编辑接口配置' : '新增接口配置'"
      width="480px"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="90px">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="例如:OpenAI 官方" maxlength="100" />
        </el-form-item>
        <el-form-item label="Base URL" prop="base_url">
          <el-input v-model="form.base_url" placeholder="例如:https://api.openai.com" />
        </el-form-item>
        <el-form-item label="API Key" prop="api_key">
          <el-input
            v-model="form.api_key"
            type="password"
            show-password
            autocomplete="new-password"
            :placeholder="editing ? '留空表示沿用原 Key' : '必填'"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="creating || updating" @click="handleSubmit">
          保存
        </el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<style scoped>
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
</style>
