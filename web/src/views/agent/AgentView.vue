<script setup lang="ts">
// Agent 配置页:列出本机支持的 Agent 工具(CodeBuddy、ZCode、WorkBuddy),
// 每个工具一个 tab;卡片内部自行管理模型清单的加载、编辑、增删与默认模型
// 设置,本页只负责 Agent 列表与还原/写回成功后的状态刷新。列表数据为页面
// 私有,由 useRequest 持有。
import { onMounted, ref, watch } from 'vue'

import AgentModelCard from '@/components/agent/AgentModelCard.vue'
import { listAgents } from '@/api/agent'
import { useRequest } from '@/composables/useRequest'

const { data: agents, loading: listLoading, run: runList } = useRequest(listAgents)

// 当前激活 tab:列表首次加载后默认定位第一个;刷新时若原 tab 仍存在则保持
const activeTab = ref('')

watch(agents, (list) => {
  if (list && list.length > 0 && !list.some((agent) => agent.name === activeTab.value)) {
    activeTab.value = list[0].name
  }
})

onMounted(() => {
  void runList()
})
</script>

<template>
  <el-card>
    <template #header>
      <div class="card-header">
        <span>Agent 配置</span>
        <el-button @click="runList()">刷新</el-button>
      </div>
    </template>

    <el-empty
      v-if="!listLoading && (agents ?? []).length === 0"
      description="暂无支持的 Agent 工具"
    />
    <!-- lazy 惰性挂载:卡片首次激活时才拉取各自的模型清单 -->
    <el-tabs v-else v-model="activeTab">
      <el-tab-pane
        v-for="agent in agents ?? []"
        :key="agent.name"
        :label="agent.display_name"
        :name="agent.name"
        lazy
      >
        <AgentModelCard
          :snapshot="agent"
          @restored="runList()"
          @default-model-changed="runList()"
          @provider-created="runList()"
        />
      </el-tab-pane>
    </el-tabs>
  </el-card>
</template>

<style scoped>
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
</style>
