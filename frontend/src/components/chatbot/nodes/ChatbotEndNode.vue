<script setup lang="ts">
import { Flag } from 'lucide-vue-next'
import BaseNode from '@/components/calling/nodes/BaseNode.vue'

defineOptions({ inheritAttrs: false })
defineProps<{ data: any }>()
</script>

<template>
  <BaseNode
    label="END"
    header-class="bg-red-600"
    :output-handles="[]"
    :has-input="true"
    :data="data"
  >
    <template #icon><Flag class="w-4 h-4" /></template>
    <div class="flex flex-col items-center py-2">
      <div
        :class="[
          'w-12 h-12 rounded-full flex items-center justify-center shadow-md transition-all',
          data?.isUnreachable ? 'bg-gray-400 opacity-50' : 'bg-red-500'
        ]"
      >
        <Flag class="h-6 w-6 text-white" />
      </div>
      <p v-if="data?.isUnreachable" class="text-[10px] text-red-600 mt-2 text-center">
        Flow has no exit path - will loop forever
      </p>
      <p v-else-if="data?.somePathsLoop" class="text-[10px] text-amber-600 mt-2 text-center">
        Some paths never reach END
      </p>
    </div>
  </BaseNode>
</template>
