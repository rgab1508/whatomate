<script setup lang="ts">
import { computed } from 'vue'
import { GitMerge } from 'lucide-vue-next'
import BaseNode from '@/components/calling/nodes/BaseNode.vue'

defineOptions({ inheritAttrs: false })

const props = defineProps<{ data: any }>()

const outputHandles = computed(() => {
  return [
    { id: 'true', label: 'True', title: 'If condition is true' },
    { id: 'false', label: 'False', title: 'If condition is false' },
  ]
})
</script>

<template>
  <BaseNode :label="data?.label || 'Branch'" header-class="bg-cyan-600" :output-handles="outputHandles" :has-input="!data?.isEntryNode" :data="data">
    <template #icon><GitMerge class="w-4 h-4" /></template>
    <div class="space-y-1">
      <div class="flex items-center gap-2">
        <div class="w-2 h-2 rounded-full bg-green-500"></div>
        <span>True path</span>
      </div>
      <div class="flex items-center gap-2">
        <div class="w-2 h-2 rounded-full bg-red-500"></div>
        <span>False path</span>
      </div>
    </div>
  </BaseNode>
</template>
