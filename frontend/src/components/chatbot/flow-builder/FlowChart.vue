<script setup lang="ts">
import { ref, watch, markRaw, nextTick, computed } from 'vue'
import { useVueFlow, MarkerType } from '@vue-flow/core'
import type { Node, Edge, NodeMouseEvent, Connection } from '@vue-flow/core'
import { Button } from '@/components/ui/button'
import {
  MessageSquare,
  MousePointerClick,
  Globe,
  MessageCircle,
  Users,
  Play,
  Flag,
  Plus,
  GitBranch,
  GitMerge,
  AlertTriangle,
  UserPlus
} from 'lucide-vue-next'
import { stepsToNodesAndEdges, extractCanvasLayout } from '@/composables/useChatbotFlowConverter'
import type { CanvasLayout } from '@/composables/useChatbotFlowConverter'
import FlowCanvas from '@/components/shared/FlowCanvas.vue'
import ChatbotTextNode from '@/components/chatbot/nodes/ChatbotTextNode.vue'
import ChatbotButtonsNode from '@/components/chatbot/nodes/ChatbotButtonsNode.vue'
import ChatbotApiNode from '@/components/chatbot/nodes/ChatbotApiNode.vue'
import ChatbotWhatsAppFlowNode from '@/components/chatbot/nodes/ChatbotWhatsAppFlowNode.vue'
import ChatbotTransferNode from '@/components/chatbot/nodes/ChatbotTransferNode.vue'
import ChatbotBranchNode from '@/components/chatbot/nodes/ChatbotBranchNode.vue'
import ChatbotEndNode from '@/components/chatbot/nodes/ChatbotEndNode.vue'

interface FlowChartStep {
  step_name: string
  step_order: number
  message: string
  message_type: string
  input_type: string
  buttons: { id: string; title: string; type?: string }[]
  conditional_next?: Record<string, string>
  next_step: string
  [key: string]: any
}

const props = defineProps<{
  steps: FlowChartStep[]
  selectedStepIndex: number | null
  flowName: string
  initialMessage: string
  completionMessage: string
  teams?: { id: string; name: string }[]
  canvasLayout?: CanvasLayout
}>()

const emit = defineEmits<{
  selectStep: [index: number]
  addStep: []
  selectFlowSettings: []
  openPreview: []
  connectSteps: [sourceStep: string, targetStep: string, sourceHandle: string]
  disconnectSteps: [sourceStep: string, sourceHandle: string]
  changeStepType: [stepIndex: number, newType: string]
  updateCanvasLayout: [layout: CanvasLayout]
}>()

const messageTypePalette = [
  { type: 'text', label: 'Text', icon: MessageSquare, color: 'bg-blue-600' },
  { type: 'buttons', label: 'Buttons', icon: MousePointerClick, color: 'bg-purple-600' },
  { type: 'api_fetch', label: 'API Fetch', icon: Globe, color: 'bg-orange-600' },
  { type: 'whatsapp_flow', label: 'WA Flow', icon: MessageCircle, color: 'bg-green-600' },
  { type: 'transfer', label: 'Transfer', icon: UserPlus, color: 'bg-amber-600' },
  { type: 'branch', label: 'Branch', icon: GitMerge, color: 'bg-cyan-600' },
]

// Track which step is selected on the canvas (index)
const selectedOnCanvas = ref<number | null>(null)

function getSelectedStepType(): string | null {
  if (selectedOnCanvas.value === null) return null
  const sorted = [...(props.steps || [])].sort((a, b) => a.step_order - b.step_order)
  return sorted[selectedOnCanvas.value]?.message_type || null
}

const nodeTypes: Record<string, any> = {
  chatbot_text: markRaw(ChatbotTextNode),
  chatbot_buttons: markRaw(ChatbotButtonsNode),
  chatbot_api: markRaw(ChatbotApiNode),
  chatbot_api_fetch: markRaw(ChatbotApiNode),
  chatbot_whatsapp_flow: markRaw(ChatbotWhatsAppFlowNode),
  chatbot_transfer: markRaw(ChatbotTransferNode),
  chatbot_branch: markRaw(ChatbotBranchNode),
  chatbot_end: markRaw(ChatbotEndNode),
}

const flowNodes = ref<Node[]>([])
const flowEdges = ref<Edge[]>([])
// Prevent rebuild while we're applying a connection change from the canvas
let skipNextRebuild = false

const { fitView } = useVueFlow()

// --- Flow Validation Logic ---
const reachableSteps = computed(() => {
  const steps = props.steps || []
  if (steps.length === 0) return new Set<number>()

  const reachable = new Set<number>()
  reachable.add(0)

  const queue = [0]
  const stepNameSet = new Set(steps.map((s) => s.step_name))

  while (queue.length > 0) {
    const currentIdx = queue.shift()!
    const step = steps[currentIdx]
    if (!step) continue

    if (step.message_type === 'transfer') continue

    const targets: string[] = []
    if (step.message_type === 'branch' && step.conditional_next) {
      if (step.conditional_next.true) targets.push(step.conditional_next.true)
      if (step.conditional_next.false) targets.push(step.conditional_next.false)
    } else if (step.message_type === 'buttons' && step.conditional_next) {
      Object.values(step.conditional_next).forEach((t) => {
        if (t) targets.push(t)
      })
    }

    if (targets.length > 0) {
      targets.forEach((targetName) => {
        const idx = steps.findIndex((s) => s.step_name === targetName)
        if (idx !== -1 && !reachable.has(idx)) {
          reachable.add(idx)
          queue.push(idx)
        }
      })
    } else {
      // Fallback
      if (step.next_step && step.next_step !== '__end__') {
        const idx = steps.findIndex((s) => s.step_name === step.next_step)
        if (idx !== -1 && !reachable.has(idx)) {
          reachable.add(idx)
          queue.push(idx)
        }
      } else if (!step.next_step && currentIdx + 1 < steps.length) {
        if (!reachable.has(currentIdx + 1)) {
          reachable.add(currentIdx + 1)
          queue.push(currentIdx + 1)
        }
      }
    }
  }
  return reachable
})

const stepsInLoop = computed(() => {
  const steps = props.steps || []
  const inLoop = new Set<number>()
  const reachable = reachableSteps.value

  for (const startIdx of reachable) {
    const visited = new Set<number>()
    const path: number[] = []

    const dfs = (currentIdx: number): boolean => {
      if (path.includes(currentIdx)) {
        const cycleStart = path.indexOf(currentIdx)
        for (let i = cycleStart; i < path.length; i++) inLoop.add(path[i])
        return true
      }
      if (visited.has(currentIdx)) return false
      visited.add(currentIdx)
      path.push(currentIdx)

      const step = steps[currentIdx]
      if (!step || step.message_type === 'transfer') {
        path.pop()
        return false
      }

      const targets: string[] = []
      if (step.message_type === 'branch' && step.conditional_next) {
        if (step.conditional_next.true) targets.push(step.conditional_next.true)
        if (step.conditional_next.false) targets.push(step.conditional_next.false)
      } else if (step.message_type === 'buttons' && step.conditional_next) {
        Object.values(step.conditional_next).forEach((t) => {
          if (t) targets.push(t)
        })
      }

      if (targets.length > 0) {
        targets.forEach((targetName) => {
          const idx = steps.findIndex((s) => s.step_name === targetName)
          if (idx !== -1) dfs(idx)
        })
      } else {
        if (step.next_step && step.next_step !== '__end__') {
          const idx = steps.findIndex((s) => s.step_name === step.next_step)
          if (idx !== -1) dfs(idx)
        } else if (!step.next_step && currentIdx + 1 < steps.length) {
          dfs(currentIdx + 1)
        }
      }

      path.pop()
      return false
    }
    dfs(startIdx)
  }
  return inLoop
})

// Check if END is reachable (no infinite loop)
const isEndReachable = computed(() => {
  const steps = props.steps || []
  if (steps.length === 0) return true
  const reachable = reachableSteps.value

  for (const stepIdx of reachable) {
    const step = steps[stepIdx]
    if (!step) continue
    if (step.message_type === 'transfer') return true

    const targets: string[] = []
    if (step.message_type === 'branch' && step.conditional_next) {
      if (step.conditional_next.true === '__end__') return true
      if (step.conditional_next.false === '__end__') return true
      if (step.conditional_next.true) targets.push(step.conditional_next.true)
      if (step.conditional_next.false) targets.push(step.conditional_next.false)
    } else if (step.message_type === 'buttons' && step.conditional_next) {
      if (Object.values(step.conditional_next).includes('__end__')) return true
      Object.values(step.conditional_next).forEach((t) => { if (t) targets.push(t) })
    }

    if (targets.length === 0) {
      if (step.next_step === '__end__') return true
      if (!step.next_step && stepIdx === steps.length - 1) return true
    }
  }
  return false
})

function rebuildGraph() {
  if (skipNextRebuild) {
    skipNextRebuild = false
    return
  }

  const steps = props.steps || []
  if (steps.length === 0) {
    flowNodes.value = []
    flowEdges.value = []
    return
  }

  const { nodes: n, edges: e } = stepsToNodesAndEdges(steps as any, props.canvasLayout)

  // Build team name lookup and inject into transfer nodes
  const teamMap = new Map<string, string>()
  if (props.teams) {
    for (const t of props.teams) {
      teamMap.set(t.id, t.name)
    }
  }
  n.forEach((node) => {
    if (node.type === 'chatbot_transfer' && node.data?.config?.transfer_config?.team_id) {
      const teamId = node.data.config.transfer_config.team_id
      node.data.config.transfer_config.team_name = teamMap.get(teamId) || ''
    }
  })

  // Mark selected and add validation status
  const sorted = [...steps].sort((a, b) => a.step_order - b.step_order)
  const reachable = reachableSteps.value
  const inLoop = stepsInLoop.value

  n.forEach((node) => {
    const stepIdx = sorted.findIndex((s) => s.step_name === node.id)
    if (node.id === '__end__') {
      node.data = {
        ...node.data,
        isUnreachable: !isEndReachable.value,
        somePathsLoop: inLoop.size > 0 && isEndReachable.value,
      }
      return
    }
    const isSelected = props.selectedStepIndex !== null && stepIdx === props.selectedStepIndex
    const isUnreachable = stepIdx > 0 && !reachable.has(stepIdx)
    const isInLoop = inLoop.has(stepIdx)

    node.data = {
      ...node.data,
      selected: isSelected,
      isUnreachable,
      isInLoop,
    }
    node.class = [
      isSelected ? 'selected-node' : '',
      isUnreachable ? 'unreachable-node' : '',
      isInLoop ? 'loop-node' : '',
    ].filter(Boolean).join(' ')
  })

  flowNodes.value = n
  flowEdges.value = e
  nextTick(() => fitView({ padding: 0.2 }))
}

// Rebuild when steps change
watch(() => props.steps, rebuildGraph, { immediate: true, deep: true })

// Update selection highlight and validation status
watch(
  () => [props.selectedStepIndex, props.steps],
  ([idx]) => {
    const steps = props.steps || []
    const sorted = [...steps].sort((a, b) => a.step_order - b.step_order)
    const reachable = reachableSteps.value
    const inLoop = stepsInLoop.value

    flowNodes.value = flowNodes.value.map((node) => {
      const stepIdx = sorted.findIndex((s) => s.step_name === node.id)
      const isSelected = idx !== null && stepIdx === idx
      const isUnreachable = stepIdx > 0 && !reachable.has(stepIdx)
      const isInLoop = inLoop.has(stepIdx)

      return {
        ...node,
        data: {
          ...node.data,
          selected: isSelected,
          isUnreachable,
          isInLoop,
        },
        class: [
          isSelected ? 'selected-node' : '',
          isUnreachable ? 'unreachable-node' : '',
          isInLoop ? 'loop-node' : '',
        ].filter(Boolean).join(' '),
      }
    })
  },
)

function onNodeClick(event: NodeMouseEvent) {
  const sorted = [...(props.steps || [])].sort((a, b) => a.step_order - b.step_order)
  const idx = sorted.findIndex((s) => s.step_name === event.node.id)
  if (idx !== -1) {
    selectedOnCanvas.value = idx
    emit('selectStep', idx)
  }
}

function onPaneClick() {
  selectedOnCanvas.value = null
  emit('selectFlowSettings')
}

function onPaletteClick(type: string) {
  if (selectedOnCanvas.value !== null) {
    emit('changeStepType', selectedOnCanvas.value, type)
  }
}

function onConnect(connection: Connection) {
  if (!connection.source || !connection.target) return
  const handle = connection.sourceHandle || 'default'

  // Remove existing edge from same source handle (enforce single connection per handle)
  flowEdges.value = flowEdges.value.filter(
    (e) => !(e.source === connection.source && e.sourceHandle === handle)
  )

  // Find button title for label
  let label = ''
  if (handle !== 'default') {
    const sourceStep = (props.steps || []).find((s) => s.step_name === connection.source)
    const btn = sourceStep?.buttons?.find((b: any) => b.id === handle)
    if (btn) label = btn.title || handle
  }

  // Add new edge
  flowEdges.value = [
    ...flowEdges.value,
    {
      id: `e-${connection.source}-${connection.target}-${handle}`,
      source: connection.source,
      target: connection.target,
      sourceHandle: handle,
      targetHandle: connection.targetHandle || undefined,
      label,
      animated: true,
      markerEnd: MarkerType.ArrowClosed,
    },
  ]

  // Tell parent to update the step data
  skipNextRebuild = true
  emit('connectSteps', connection.source, connection.target, handle)
}

function onNodeDragStop() {
  emit('updateCanvasLayout', extractCanvasLayout(flowNodes.value))
}

function onEdgeRemove(edges: Edge[]) {
  for (const edge of edges) {
    const handle = edge.sourceHandle || 'default'
    skipNextRebuild = true
    emit('disconnectSteps', edge.source, handle)
  }
}
</script>

<template>
  <div class="h-full flex flex-col overflow-hidden">
    <!-- Header / Toolbar -->
    <div class="px-4 py-3 border-b flex items-center justify-between flex-shrink-0">
      <div class="flex items-center gap-2">
        <GitBranch class="h-4 w-4 text-muted-foreground" />
        <span class="text-sm font-medium">Flow Diagram</span>
      </div>
      <div class="flex items-center gap-2">
        <Button variant="outline" size="sm" @click="emit('openPreview')">
          <Play class="h-4 w-4 mr-1" />
          Preview
        </Button>
        <Button variant="outline" size="sm" @click="emit('addStep')">
          <Plus class="h-4 w-4 mr-1" />
          Add Step
        </Button>
      </div>
    </div>

    <!-- Message Type Palette -->
    <div class="flex items-center gap-2 px-4 py-2 border-b bg-muted/30 overflow-x-auto shrink-0">
      <span class="text-xs text-muted-foreground shrink-0">
        {{ selectedOnCanvas !== null ? 'Change type:' : 'Add step:' }}
      </span>
      <Button
        v-for="p in messageTypePalette"
        :key="p.type"
        :variant="getSelectedStepType() === p.type ? 'active' : 'outline'"
        size="sm"
        class="h-7 text-xs gap-1.5 shrink-0"
        @click="selectedOnCanvas !== null ? onPaletteClick(p.type) : emit('addStep')"
      >
        <div :class="['w-2 h-2 rounded-full', p.color]" />
        <component :is="p.icon" class="w-3.5 h-3.5" />
        {{ p.label }}
      </Button>
    </div>

    <!-- Vue Flow Canvas -->
    <div class="flex-1 relative">
      <FlowCanvas
        :nodes="flowNodes"
        :edges="flowEdges"
        :node-types="nodeTypes"
        edge-type="default"
        fit-view-on-init
        @update:nodes="flowNodes = $event"
        @update:edges="flowEdges = $event"
        @node-click="onNodeClick"
        @node-drag-stop="onNodeDragStop"
        @pane-click="onPaneClick"
        @connect="onConnect"
        @edges-change="(changes) => {
          const removals = changes.filter((c: any) => c.type === 'remove')
          if (removals.length) onEdgeRemove(removals.map((r: any) => flowEdges.find((e) => e.id === r.id)).filter(Boolean) as Edge[])
        }"
      />

      <!-- Empty state overlay -->
      <div
        v-if="steps.length === 0"
        class="absolute inset-0 flex items-center justify-center pointer-events-none z-10"
      >
        <div
          class="w-72 py-12 rounded-xl border-2 border-dashed border-gray-300 dark:border-gray-600 flex flex-col items-center justify-center cursor-pointer hover:border-primary hover:bg-primary/5 transition-all pointer-events-auto shadow-sm bg-white/50 dark:bg-black/20"
          @click="emit('addStep')"
        >
          <Plus class="h-10 w-10 text-gray-400 mb-3" />
          <span class="text-sm font-medium text-muted-foreground">Add your first step</span>
        </div>
      </div>
    </div>
  </div>
</template>
