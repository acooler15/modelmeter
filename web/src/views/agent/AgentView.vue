<script setup lang="ts">
// Agent 配置页:列出本机支持的 Agent 工具(ZCode、WorkBuddy),每个工具
// 一张卡片;卡片内部自行管理模型清单的加载、编辑、默认模型设置与保存,本页
// 只负责 Agent 列表与还原/默认模型写回成功后的状态刷新。列表数据为页面私有,
// 由 useRequest 持有。
import { onMounted } from 'vue'

import AgentModelCard from '@/components/agent/AgentModelCard.vue'
import { listAgents } from '@/api/agent'
import { useRequest } from '@/composables/useRequest'

const { data: agents, loading: listLoading, run: runList } = useRequest(listAgents)

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
    <div v-else class="agent-list">
      <AgentModelCard
        v-for="agent in agents ?? []"
        :key="agent.name"
        :snapshot="agent"
        @restored="runList()"
        @default-model-changed="runList()"
      />
    </div>
  </el-card>
</template>

<style scoped>
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.agent-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
</style>
