<script setup lang="ts">
// Agent 配置页:展示本机 Agent 工具(ZCode、WorkBuddy)的模型配置现状,
// 支持在线修改(后端写回前自动备份原配置)与一键还原;未找到配置的工具
// 展示中文指引且按钮禁用。列表数据为页面私有,由 useRequest 持有,
// 错误提示统一由 useRequest 弹出。
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'

import { applyAgent, listAgents, restoreAgent } from '@/api/agent'
import { useRequest } from '@/composables/useRequest'
import type { AgentSnapshot } from '@/types/agent'

const { data: agents, loading: listLoading, run: runList } = useRequest(listAgents)
const { run: runApply, loading: applying } = useRequest(applyAgent)
const { run: runRestore, loading: restoring } = useRequest(restoreAgent)

// 修改配置对话框状态:editing 为当前操作的 Agent 快照,form 为字段草稿
const dialogVisible = ref(false)
const editing = ref<AgentSnapshot | null>(null)
const formRef = ref<FormInstance>()
const form = reactive<Record<string, string>>({})

// 按后端字段描述动态生成必填校验规则,提示文案全中文
const rules = computed<FormRules>(() => {
  const result: FormRules = {}
  for (const field of editing.value?.fields ?? []) {
    if (field.required) {
      result[field.key] = [{ required: true, message: `请填写${field.label}`, trigger: 'blur' }]
    }
  }
  return result
})

/** 打开修改对话框:用当前值初始化表单草稿并清理上一次的校验状态。 */
function openDialog(agent: AgentSnapshot) {
  editing.value = agent
  Object.keys(form).forEach((key) => delete form[key])
  for (const field of agent.fields) {
    form[field.key] = agent.values[field.key] ?? ''
  }
  dialogVisible.value = true
  void nextTick(() => formRef.value?.clearValidate())
}

/** 提交修改:二次确认后写回;后端会先自动备份原配置。 */
async function handleSubmit() {
  if (!editing.value || !formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  try {
    await ElMessageBox.confirm(
      `确定修改「${editing.value.display_name}」的模型配置吗?修改前会自动备份原配置。`,
      '修改确认',
      { type: 'warning', confirmButtonText: '保存', cancelButtonText: '取消' },
    )
  } catch {
    return // 用户取消,无需提示
  }
  const snap = await runApply(editing.value.name, { ...form })
  if (snap === undefined) return // 失败已由 useRequest 统一提示
  ElMessage.success('保存成功,原配置已自动备份')
  dialogVisible.value = false
  void runList()
}

/** 从最近一份备份还原:二次确认后调用还原接口并刷新列表。 */
async function handleRestore(agent: AgentSnapshot) {
  try {
    await ElMessageBox.confirm(
      `确定将「${agent.display_name}」的配置还原到最近一份备份吗?`,
      '还原确认',
      { type: 'warning', confirmButtonText: '还原', cancelButtonText: '取消' },
    )
  } catch {
    return // 用户取消,无需提示
  }
  const snap = await runRestore(agent.name)
  if (snap === undefined) return
  ElMessage.success('已从最近一份备份还原')
  void runList()
}

/** 当前值摘要:按字段 Label 拼接,空值显示占位文案。 */
function summary(agent: AgentSnapshot): string[] {
  return agent.fields.map((field) => `${field.label}:${agent.values[field.key] || '未设置'}`)
}

onMounted(() => {
  void runList()
})
</script>

<template>
  <el-card v-loading="listLoading">
    <template #header>
      <div class="card-header">
        <span>Agent 配置</span>
        <el-button @click="runList()">刷新</el-button>
      </div>
    </template>

    <el-row :gutter="16">
      <el-col v-for="agent in agents ?? []" :key="agent.name" :span="12" class="agent-col">
        <el-card shadow="never" class="agent-card">
          <div class="agent-title">
            <span class="agent-name">{{ agent.display_name }}</span>
            <el-tag :type="agent.status === 'found' ? 'success' : 'info'">
              {{ agent.status === 'found' ? '已找到' : '未找到' }}
            </el-tag>
          </div>
          <div class="agent-path">配置文件:{{ agent.config_path || '未知' }}</div>

          <el-alert
            v-if="agent.message"
            :title="agent.message"
            :type="agent.status === 'found' ? 'info' : 'warning'"
            :closable="false"
            class="agent-message"
          />

          <ul v-if="agent.fields.length > 0" class="agent-values">
            <li v-for="line in summary(agent)" :key="line">{{ line }}</li>
          </ul>

          <div class="agent-actions">
            <el-button
              type="primary"
              :disabled="agent.status !== 'found' || agent.fields.length === 0"
              @click="openDialog(agent)"
            >
              修改配置
            </el-button>
            <el-button :disabled="!agent.config_path" :loading="restoring" @click="handleRestore(agent)">
              还原上次备份
            </el-button>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-dialog
      v-model="dialogVisible"
      :title="`修改${editing?.display_name ?? ''}模型配置`"
      width="520px"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
        <el-form-item
          v-for="field in editing?.fields ?? []"
          :key="field.key"
          :label="field.label"
          :prop="field.key"
        >
          <el-select v-if="field.type === 'select'" v-model="form[field.key]" class="full-width">
            <el-option
              v-for="opt in field.options ?? []"
              :key="opt.value"
              :label="opt.label"
              :value="opt.value"
            />
          </el-select>
          <el-input v-else v-model="form[field.key]" />
          <div v-if="field.help" class="field-help">{{ field.help }}</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="applying" @click="handleSubmit">保存</el-button>
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

.agent-col {
  margin-bottom: 16px;
}

.agent-card {
  height: 100%;
}

.agent-title {
  display: flex;
  align-items: center;
  gap: 8px;
}

.agent-name {
  font-size: 16px;
  font-weight: 600;
}

.agent-path {
  margin-top: 8px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
  word-break: break-all;
}

.agent-message {
  margin-top: 12px;
}

.agent-values {
  margin: 12px 0 0;
  padding-left: 18px;
  color: var(--el-text-color-regular);
}

.agent-actions {
  margin-top: 16px;
}

.full-width {
  width: 100%;
}

.field-help {
  width: 100%;
  margin-top: 4px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--el-text-color-secondary);
}
</style>
