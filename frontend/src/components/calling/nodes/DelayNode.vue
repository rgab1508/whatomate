<script setup lang="ts">
import { computed } from 'vue'
import { Timer } from 'lucide-vue-next'
import BaseNode from './BaseNode.vue'

defineOptions({ inheritAttrs: false })

const props = defineProps<{ data: Record<string, any> }>()

const seconds = computed(() => Number(props.data?.config?.seconds) || 2)

const outputHandles = [{ id: 'default', label: 'next' }]
</script>

<template>
  <BaseNode :label="data?.label || 'Delay'" header-class="bg-slate-600" :output-handles="outputHandles" :has-input="!data?.isEntryNode">
    <template #icon><Timer class="w-4 h-4" /></template>
    <p>Wait {{ seconds }}s</p>
  </BaseNode>
</template>
