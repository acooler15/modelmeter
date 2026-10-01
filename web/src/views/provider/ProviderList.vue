<script setup lang="ts">
// 接口配置页:管理站点名称、Base URL 与 API Key 的增删改查。
// 列表默认打码展示 Key(maskKey 仅展示层,数据仍是明文),每行可切换显示;
// 明文态点击 Key 文本即复制完整值。编辑弹窗回填当前明文 Key,密码框圆点
// 隐藏、点眼睛查看;清空提交则由后端沿用原 Key。
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
import { maskKey } from '@/utils/mask'

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
    'api_key' in x &&
    'updated_at' in x
  )
}

// 行级明文显示状态:按 id 记录处于"显示"态的行,切换只影响该行。
// 整体替换 Set 而非原地 add/delete,保证 ref 的响应式触发可靠。
const revealedIds = ref<ReadonlySet<number>>(new Set())

/** 该行是否处于明文显示态。 */
function isRevealed(row: unknown): boolean {
  return isProviderRow(row) && revealedIds.value.has(row.id)
}

/** 切换某行的显示/隐藏;调用方(模板)需 @click.stop 防止事件冒泡。 */
function toggleRevealed(row: unknown) {
  if (!isProviderRow(row)) return
  const next = new Set(revealedIds.value)
  if (next.has(row.id)) {
    next.delete(row.id)
  } else {
    next.add(row.id)
  }
  revealedIds.value = next
}

/** Key 单元格展示文本:显示态返回明文,否则返回打码值。
 * 刷新后 revealedIds 已清空,新数据自然回到打码态。 */
function keyCellText(row: unknown): string {
  if (!isProviderRow(row)) return ''
  return revealedIds.value.has(row.id) ? row.api_key : maskKey(row.api_key)
}

/** 明文态点击 Key 文本复制完整 Key;打码态不响应点击。 */
function copyRowKey(row: unknown) {
  if (!isProviderRow(row)) return
  if (!revealedIds.value.has(row.id)) return
  void copyKey(row.api_key)
}

/** 复制到剪贴板:优先 Clipboard API(仅安全上下文可用),调用失败或
 * 不可用(如 HTTP 本地部署)时降级为隐藏 textarea + execCommand('copy')。
 * writeText 被拒最常见的原因是点击瞬间文档尚未聚焦(如从其他窗口切回
 * 立即点击),聚焦后重试一次即可恢复,避免误报失败。 */
async function copyKey(key: string) {
  let copied = false
  if (window.isSecureContext && navigator.clipboard) {
    // Clipboard API 失败(权限被拒等)时降级,不直接打断复制流程
    const write = () =>
      navigator.clipboard.writeText(key).then(
        () => true,
        () => false,
      )
    copied = await write()
    if (!copied) {
      window.focus()
      copied = await write()
    }
  }
  if (!copied) copied = copyViaExecCommand(key)
  if (copied) {
    ElMessage.success('API Key 已复制')
  } else {
    ElMessage.error('复制失败,请手动选择 Key 后复制')
  }
}

/** 降级复制:临时挂载隐藏 textarea,选中后借 execCommand('copy') 完成。
 * 返回是否成功,提示由调用方统一给出。 */
function copyViaExecCommand(text: string): boolean {
  const textarea = document.createElement('textarea')
  textarea.value = text
  // 移出可视区域而非 display:none:部分浏览器会拒绝复制不可聚焦的元素
  textarea.style.position = 'fixed'
  textarea.style.top = '-9999px'
  document.body.appendChild(textarea)
  // 先聚焦再选中:聚焦会连带提升文档焦点,部分浏览器要求文档处于聚焦态
  textarea.focus()
  textarea.select()
  let copied = false
  try {
    copied = document.execCommand('copy')
  } finally {
    document.body.removeChild(textarea)
  }
  return copied
}

/** 打开对话框并重置表单:row 为 null 表示新增。
 * 编辑时回填当前明文 Key,配合密码框圆点隐藏、点眼睛查看;
 * 用户清空后提交则由后端沿用原 Key,语义与"空则沿用"规则一致。 */
function openDialog(row: unknown) {
  const target = isProviderRow(row) ? row : null
  editing.value = target
  Object.assign(form, {
    name: target?.name ?? '',
    base_url: target?.base_url ?? '',
    api_key: target?.api_key ?? '',
  })
  dialogVisible.value = true
  // 清除上一次打开遗留的校验状态
  void nextTick(() => formRef.value?.clearValidate())
}

/** 写操作成功后刷新列表,并在刷新完成后把所有行切回打码态:
 * 列表数据即将被替换,残留的显示态可能对应已被修改或删除的旧 Key。
 * load 内部吞错不会 reject,链 then 是安全的。 */
function refreshAndResetRevealed() {
  void providersStore.load(true).then(() => {
    revealedIds.value = new Set()
  })
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
  refreshAndResetRevealed()
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
  refreshAndResetRevealed()
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
      <!-- Key 列不用 show-overflow-tooltip:单元格内含点击复制文本与切换
           按钮,避免 tooltip 触发区域干扰点击交互;打码值长度固定,明文态
           由 word-break 换行兜底 -->
      <el-table-column label="API Key" min-width="220">
        <template #default="{ row }">
          <div class="key-cell">
            <span
              v-if="isRevealed(row)"
              class="key-plaintext"
              title="点击复制完整 Key"
              @click="copyRowKey(row)"
            >{{ keyCellText(row) }}</span>
            <span v-else>{{ keyCellText(row) }}</span>
            <el-button link type="primary" @click.stop="toggleRevealed(row)">
              {{ isRevealed(row) ? '隐藏' : '显示' }}
            </el-button>
          </div>
        </template>
      </el-table-column>
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
            :placeholder="editing ? '清空提交则沿用原 Key' : '必填'"
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
.key-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}
/* 明文态提示可点击复制:悬停加下划线,长 Key 允许换行不溢出 */
.key-plaintext {
  cursor: pointer;
  word-break: break-all;
}
.key-plaintext:hover {
  text-decoration: underline;
}
</style>
