<script setup lang="ts">
import { computed } from 'vue'
import { GitBranch } from 'lucide-vue-next'
import BaseNode from './BaseNode.vue'

defineOptions({ inheritAttrs: false })

const props = defineProps<{ data: Record<string, any> }>()

const summary = computed(() => {
  const expression = props.data?.config?.expression as string | undefined
  if (!expression) return 'No expression'
  return expression.length > 60 ? expression.slice(0, 60) + '…' : expression
})

const outputHandles = [
  { id: 'true', label: 'True', title: 'Expression is true' },
  { id: 'false', label: 'False', title: 'Expression is false, or could not be evaluated' },
]
</script>

<template>
  <BaseNode :label="data?.label || 'Condition'" header-class="bg-indigo-600" :output-handles="outputHandles" :has-input="!data?.isEntryNode">
    <template #icon><GitBranch class="w-4 h-4" /></template>
    <p class="truncate font-mono text-[10px]" :title="summary">{{ summary }}</p>
  </BaseNode>
</template>
