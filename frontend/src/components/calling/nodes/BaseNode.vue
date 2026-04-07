<script setup lang="ts">
import { computed } from 'vue'
import { Handle, Position } from '@vue-flow/core'
import { AlertTriangle } from 'lucide-vue-next'

const props = withDefaults(
  defineProps<{
    label: string
    headerClass: string
    hasInput?: boolean
    outputHandles?: { id: string; label: string; title?: string }[]
    data?: any
  }>(),
  { hasInput: true },
)

const gradientMap: Record<string, string> = {
  'bg-blue-600': 'from-blue-600 to-blue-500',
  'bg-purple-600': 'from-purple-600 to-purple-500',
  'bg-orange-600': 'from-orange-600 to-amber-500',
  'bg-green-600': 'from-green-600 to-emerald-500',
  'bg-amber-600': 'from-amber-600 to-yellow-500',
  'bg-red-600': 'from-red-600 to-rose-500',
  'bg-cyan-600': 'from-cyan-600 to-cyan-500',
  'bg-teal-600': 'from-teal-600 to-teal-500',
}

const headerGradient = computed(() => gradientMap[props.headerClass] || props.headerClass)
</script>

<template>
  <div
    :class="[
      'base-node relative bg-background border rounded-lg shadow-md hover:shadow-lg min-w-48 w-max max-w-sm overflow-visible transition-shadow duration-200',
      data?.isUnreachable ? 'border-red-400 bg-red-50 dark:bg-red-900/20 opacity-80' : '',
      data?.isInLoop ? 'border-amber-400 bg-amber-50 dark:bg-amber-900/20' : '',
    ]"
  >
    <!-- Loop Warning Badge -->
    <div
      v-if="data?.isInLoop && !data?.isUnreachable"
      class="absolute -top-3 left-1/2 -translate-x-1/2 bg-amber-500 text-white text-[10px] font-bold px-3 py-1 rounded-full flex items-center gap-1 shadow-md z-[20]"
    >
      <svg class="h-3 w-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M21 12a9 9 0 1 1-6.219-8.56" />
      </svg>
      LOOP
    </div>
    <!-- Unreachable Warning Badge -->
    <div
      v-if="data?.isUnreachable"
      class="absolute -top-3 left-1/2 -translate-x-1/2 bg-red-500 text-white text-[10px] font-bold px-3 py-1 rounded-full flex items-center gap-1 shadow-md z-[20]"
    >
      <AlertTriangle class="h-3 w-3" />
      UNREACHABLE
    </div>
    <!-- Entry Node Badge -->
    <div
      v-if="data?.isEntryNode"
      class="absolute -top-3 left-1/2 -translate-x-1/2 bg-green-500 text-white text-[10px] font-bold px-3 py-1 rounded-full flex items-center gap-1 shadow-md z-[20]"
    >
      START
    </div>

    <!-- Input handle (top) -->
    <Handle
      v-if="hasInput !== false"
      id="input"
      type="target"
      :position="Position.Top"
      class="!w-3.5 !h-3.5 !rounded-full !bg-slate-400 !border-2 !border-background hover:!bg-slate-300 !transition-colors"
      style="z-index: 10;"
    />

    <!-- Header -->
    <div :class="['px-3 py-2 rounded-t-lg text-white text-xs font-semibold flex items-center gap-2 overflow-hidden bg-gradient-to-r', headerGradient]">
      <slot name="icon" />
      <span class="truncate flex-1">{{ label }}</span>
      <span v-if="data?.index !== undefined" class="opacity-70 text-[10px] bg-white/20 px-1.5 rounded-full">{{ data.index + 1 }}</span>
    </div>

    <!-- Body -->
    <div class="px-3 py-2.5 text-xs text-muted-foreground">
      <slot />
    </div>

    <!-- Output handles (bottom) -->
    <template v-if="outputHandles && outputHandles.length > 0">
      <Handle
        v-for="(handle, idx) in outputHandles"
        :key="handle.id"
        type="source"
        :id="handle.id"
        :position="Position.Bottom"
        :title="handle.title || handle.label"
        :style="{
          left: outputHandles.length === 1 ? '50%' : `${((idx + 1) / (outputHandles.length + 1)) * 100}%`,
          zIndex: 10,
        }"
        class="!w-3.5 !h-3.5 !rounded-full !bg-primary !border-2 !border-background hover:!bg-primary/80 !transition-colors"
      />
      <span
        v-for="(handle, idx) in outputHandles"
        :key="'num-' + handle.id"
        class="absolute text-[9px] font-bold text-muted-foreground pointer-events-none"
        :style="{
          left: outputHandles.length === 1 ? '50%' : `${((idx + 1) / (outputHandles.length + 1)) * 100}%`,
          bottom: '-18px',
          transform: 'translateX(-50%)',
        }"
      >{{ idx + 1 }}</span>
    </template>
    <template v-else-if="!outputHandles">
      <Handle
        id="default"
        type="source"
        :position="Position.Bottom"
        class="!w-3.5 !h-3.5 !rounded-full !bg-primary !border-2 !border-background hover:!bg-primary/80 !transition-colors"
        style="z-index: 10;"
      />
    </template>
  </div>
</template>
