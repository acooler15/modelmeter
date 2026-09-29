<script setup lang="ts">
// 首页:展示后端健康检查结果,验证前后端整条链路
import { onMounted } from 'vue'

import { getHealth } from '@/api/health'
import { useRequest } from '@/composables/useRequest'

const { data: health, loading, run } = useRequest(getHealth)

// 组件挂载时发起首次探活
onMounted(() => {
  void run()
})
</script>

<template>
  <el-card>
    <template #header>服务状态</template>
    <el-skeleton v-if="loading" :rows="1" animated />
    <el-result
      v-else-if="health"
      icon="success"
      title="服务运行正常"
      sub-title="后端健康检查通过,数据库连接可用"
    />
    <el-empty v-else description="暂时无法获取服务状态" />
  </el-card>
</template>
