<script setup lang="ts">
import { computed } from 'vue'
import { Grid3X3 } from 'lucide-vue-next'
import BaseNode from './BaseNode.vue'

defineOptions({ inheritAttrs: false })

const props = defineProps<{ data: Record<string, any> }>()

const isDynamic = computed(() => !!props.data?.config?.dynamic_options)
const optionsVariableRef = computed(() => `{{${props.data?.config?.options_variable || '?'}}}`)

const options = computed(() => {
  const opts = props.data?.config?.options || {}
  return Object.entries(opts).sort(([a], [b]) => a.localeCompare(b))
})

const outputHandles = computed(() => {
  const handles: { id: string; label: string; title?: string }[] = []
  if (isDynamic.value) {
    handles.push({ id: 'selected', label: 'Picked', title: 'Caller picked one of the options' })
    handles.push({ id: 'no_options', label: 'None', title: 'No options available - the menu is skipped' })
    handles.push({ id: 'timeout', label: 'T/O', title: 'Timed Out - no key pressed (falls back to Max if not connected)' })
    handles.push({ id: 'max_retries', label: 'Max', title: 'Max Retries - too many invalid attempts' })
    return handles
  }
  for (const [digit] of options.value) {
    handles.push({ id: `digit:${digit}`, label: digit })
  }
  handles.push({ id: 'timeout', label: 'T/O', title: 'Timed Out - no key pressed (falls back to Max if not connected)' })
  handles.push({ id: 'max_retries', label: 'Max', title: 'Max Retries - too many invalid attempts' })
  return handles
})
</script>

<template>
  <BaseNode :label="data?.label || 'Menu'" header-class="bg-purple-600" :output-handles="outputHandles" :has-input="!data?.isEntryNode">
    <template #icon><Grid3X3 class="w-4 h-4" /></template>
    <p v-if="isDynamic" class="text-muted-foreground">
      Options from <span class="font-mono">{{ optionsVariableRef }}</span>
    </p>
    <div v-else-if="options.length > 0" class="space-y-0.5">
      <div v-for="[digit, opt] in options" :key="digit" class="flex gap-1">
        <span class="font-mono font-bold">{{ digit }}:</span>
        <span class="truncate">{{ (opt as any)?.label || '—' }}</span>
      </div>
    </div>
    <p v-else class="text-muted-foreground">No options</p>
  </BaseNode>
</template>
