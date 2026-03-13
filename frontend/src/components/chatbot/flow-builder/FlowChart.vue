<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { Badge } from '@/components/ui/badge'
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
  AlertTriangle
} from 'lucide-vue-next'

interface ButtonConfig {
  id: string
  title: string
  type?: 'reply' | 'url' | 'phone'
  url?: string
  phone_number?: string
}

interface FlowStep {
  id?: string
  step_name: string
  step_order: number
  message: string
  message_type: string
  input_type: string
  buttons: ButtonConfig[]
  conditional_next?: Record<string, string>
  next_step: string
}

const props = defineProps<{
  steps: FlowStep[]
  selectedStepIndex: number | null
  flowName: string
  initialMessage: string
  completionMessage: string
}>()

const emit = defineEmits<{
  selectStep: [index: number]
  addStep: []
  selectFlowSettings: []
  openPreview: []
}>()

// Refs for measuring positions
const containerRef = ref<HTMLElement | null>(null)
const stepRefs = ref<Map<number, HTMLElement>>(new Map())
const buttonRefs = ref<Map<string, HTMLElement>>(new Map())
const startRef = ref<HTMLElement | null>(null)
const endRef = ref<HTMLElement | null>(null)
const svgSize = ref({ width: 1200, height: 2000 })

// Connection lines data
const connections = ref<Array<{
  path: string
  color: string
  label: string
  labelX: number
  labelY: number
}>>([])

const messageTypeIcons: Record<string, any> = {
  text: MessageSquare,
  buttons: MousePointerClick,
  api_fetch: Globe,
  whatsapp_flow: MessageCircle,
  transfer: Users,
  branch: GitMerge
}

const messageTypeColors: Record<string, string> = {
  text: 'bg-blue-500',
  buttons: 'bg-purple-500',
  api_fetch: 'bg-orange-500',
  whatsapp_flow: 'bg-green-500',
  transfer: 'bg-amber-500',
  branch: 'bg-cyan-500'
}

const lineColors = [
  '#8b5cf6', // purple
  '#06b6d4', // cyan
  '#f59e0b', // amber
  '#10b981', // emerald
  '#ec4899', // pink
]

function getStepIcon(messageType: string) {
  return messageTypeIcons[messageType] || MessageSquare
}

function getStepColor(messageType: string) {
  return messageTypeColors[messageType] || 'bg-gray-500'
}

function setStepRef(el: any, idx: number) {
  if (el) stepRefs.value.set(idx, el)
}

function setButtonRef(el: any, stepIdx: number, btnIdx: number) {
  if (el) buttonRefs.value.set(`${stepIdx}-${btnIdx}`, el)
}

// Get button destination
function getButtonDestination(step: FlowStep, stepIdx: number, btn: ButtonConfig, btnIdx: number) {
  const buttonId = btn.id || `btn_${btnIdx + 1}`
  const targetStepName = step.conditional_next?.[buttonId]

  if (targetStepName) {
    if (targetStepName === '__end__') {
      return { targetIdx: -1, targetName: 'End' }
    }
    const targetIdx = props.steps.findIndex(s => s.step_name === targetStepName)
    if (targetIdx !== -1) {
      return { targetIdx, targetName: targetStepName }
    }
  }

  // Default: next sequential step
  const nextIdx = stepIdx + 1
  if (nextIdx < props.steps.length) {
    return { targetIdx: nextIdx, targetName: props.steps[nextIdx].step_name || `Step ${nextIdx + 1}` }
  }

  return { targetIdx: -1, targetName: 'End' } // -1 = End
}

// Get reply buttons for a step
function getReplyButtons(step: FlowStep) {
  return step.buttons?.filter(b => b.type !== 'url') || []
}

// Check if step has buttons
function hasButtons(step: FlowStep) {
  return step.message_type === 'buttons' && getReplyButtons(step).length > 0
}

// Check if step is a branch
function isBranch(step: FlowStep) {
  return step.message_type === 'branch'
}

// Check if a step is a direct target of any branch step (should not be reached via sequential fallthrough)
function isBranchTargetStep(stepName: string): boolean {
  return props.steps.some(s =>
    s.message_type === 'branch' &&
    s.conditional_next &&
    (s.conditional_next['true'] === stepName || s.conditional_next['false'] === stepName)
  )
}

// Get branch destinations (true/false paths)
function getBranchDestinations(step: FlowStep, stepIdx: number): { trueDest: { targetIdx: number; targetName: string }; falseDest: { targetIdx: number; targetName: string } } {
  const trueStepName = step.conditional_next?.['true']
  const falseStepName = step.conditional_next?.['false']

  const resolveTarget = (name: string | undefined) => {
    if (name) {
      if (name === '__end__') return { targetIdx: -1, targetName: 'End' }
      const idx = props.steps.findIndex(s => s.step_name === name)
      if (idx !== -1) return { targetIdx: idx, targetName: name }
    }
    // Default: next sequential step
    const nextIdx = stepIdx + 1
    if (nextIdx < props.steps.length) {
      return { targetIdx: nextIdx, targetName: props.steps[nextIdx].step_name || `Step ${nextIdx + 1}` }
    }
    return { targetIdx: -1, targetName: 'End' }
  }

  return {
    trueDest: resolveTarget(trueStepName),
    falseDest: resolveTarget(falseStepName)
  }
}

// Calculate reachable steps
const reachableSteps = computed(() => {
  const reachable = new Set<number>()
  reachable.add(0) // First step is always reachable

  // BFS to find all reachable steps
  const queue = [0]
  while (queue.length > 0) {
    const currentIdx = queue.shift()!
    const step = props.steps[currentIdx]
    if (!step) continue

    // Transfer steps end the flow - nothing after is reachable from this path
    if (step.message_type === 'transfer') {
      // Transfer ends flow, don't add next step
      continue
    }

    if (isBranch(step)) {
      // For branch steps, both true and false targets are reachable
      const { trueDest, falseDest } = getBranchDestinations(step, currentIdx)
      for (const dest of [trueDest, falseDest]) {
        if (dest.targetIdx >= 0 && !reachable.has(dest.targetIdx)) {
          reachable.add(dest.targetIdx)
          queue.push(dest.targetIdx)
        }
      }
    } else if (hasButtons(step)) {
      // For button steps, only targets of buttons are reachable
      const buttons = getReplyButtons(step)
      buttons.forEach((btn, btnIdx) => {
        const dest = getButtonDestination(step, currentIdx, btn, btnIdx)
        if (dest.targetIdx >= 0 && !reachable.has(dest.targetIdx)) {
          reachable.add(dest.targetIdx)
          queue.push(dest.targetIdx)
        }
      })
    } else {
      // For non-button steps, follow explicit next_step or sequential fallback
      const step = props.steps[currentIdx]
      if (step.next_step && step.next_step !== '__end__') {
        const targetIdx = props.steps.findIndex(s => s.step_name === step.next_step)
        if (targetIdx >= 0 && !reachable.has(targetIdx)) {
          reachable.add(targetIdx)
          queue.push(targetIdx)
        }
      } else if (!step.next_step) {
        // Sequential fallback (unless it's a branch target)
        const nextIdx = currentIdx + 1
        if (nextIdx < props.steps.length && !reachable.has(nextIdx)) {
          const nextStepName = props.steps[nextIdx].step_name
          if (!isBranchTargetStep(nextStepName)) {
            reachable.add(nextIdx)
            queue.push(nextIdx)
          }
        }
      }
      // If next_step === '__end__', no further steps are reachable from here
    }
  }

  return reachable
})

// Check if a step is unreachable
function isUnreachable(stepIdx: number): boolean {
  return stepIdx > 0 && !reachableSteps.value.has(stepIdx)
}

// Check if END is reachable (no infinite loop)
const isEndReachable = computed(() => {
  if (props.steps.length === 0) return true

  // Check each reachable step to see if any path leads to END
  for (const stepIdx of reachableSteps.value) {
    const step = props.steps[stepIdx]
    if (!step) continue

    // Transfer steps end the flow (reach "end" via human handoff)
    if (step.message_type === 'transfer') {
      return true
    }

    if (isBranch(step)) {
      // Check if either branch path leads to END
      const { trueDest, falseDest } = getBranchDestinations(step, stepIdx)
      if (trueDest.targetIdx === -1 || falseDest.targetIdx === -1) {
        return true
      }
    } else if (hasButtons(step)) {
      // Check if any button leads to END
      const buttons = getReplyButtons(step)
      for (let btnIdx = 0; btnIdx < buttons.length; btnIdx++) {
        const dest = getButtonDestination(step, stepIdx, buttons[btnIdx], btnIdx)
        if (dest.targetIdx === -1) {
          return true // This button goes to END
        }
      }
    } else {
      // Non-button step: check explicit __end__, last step, or branch-blocked
      if (step.next_step === '__end__') {
        return true
      }
      const nextIdx = stepIdx + 1
      const nextIsBranchTarget = nextIdx < props.steps.length && isBranchTargetStep(props.steps[nextIdx].step_name)
      if (stepIdx === props.steps.length - 1 || nextIsBranchTarget) {
        return true
      }
    }
  }

  return false // No path to END found - it's a loop!
})

// Detect steps that are part of a loop (cycle detection)
const stepsInLoop = computed(() => {
  const inLoop = new Set<number>()

  // For each reachable step, check if it can reach itself
  for (const startIdx of reachableSteps.value) {
    const visited = new Set<number>()
    const path: number[] = []

    function dfs(currentIdx: number): boolean {
      if (path.includes(currentIdx)) {
        // Found a cycle - mark all steps in the cycle
        const cycleStart = path.indexOf(currentIdx)
        for (let i = cycleStart; i < path.length; i++) {
          inLoop.add(path[i])
        }
        return true
      }

      if (visited.has(currentIdx)) return false
      visited.add(currentIdx)
      path.push(currentIdx)

      const step = props.steps[currentIdx]
      if (!step || step.message_type === 'transfer') {
        path.pop()
        return false
      }

      if (isBranch(step)) {
        const { trueDest, falseDest } = getBranchDestinations(step, currentIdx)
        if (trueDest.targetIdx >= 0) dfs(trueDest.targetIdx)
        if (falseDest.targetIdx >= 0) dfs(falseDest.targetIdx)
      } else if (hasButtons(step)) {
        const buttons = getReplyButtons(step)
        for (let btnIdx = 0; btnIdx < buttons.length; btnIdx++) {
          const dest = getButtonDestination(step, currentIdx, buttons[btnIdx], btnIdx)
          if (dest.targetIdx >= 0) {
            dfs(dest.targetIdx)
          }
        }
      } else {
        const nextIdx = currentIdx + 1
        if (nextIdx < props.steps.length && !isBranchTargetStep(props.steps[nextIdx].step_name)) {
          dfs(nextIdx)
        }
      }

      path.pop()
      return false
    }

    dfs(startIdx)
  }

  return inLoop
})

// Check if a step is part of a loop
function isInLoop(stepIdx: number): boolean {
  return stepsInLoop.value.has(stepIdx)
}

// Calculate and draw connection lines
function updateConnections() {
  if (!containerRef.value) return

  const container = containerRef.value
  const containerRect = container.getBoundingClientRect()
  const scrollLeft = container.scrollLeft
  const scrollTop = container.scrollTop

  const newConnections: typeof connections.value = []
  let colorIdx = 0

  // Update SVG size based on content
  const contentHeight = container.scrollHeight
  const contentWidth = container.scrollWidth
  svgSize.value = { width: Math.max(1200, contentWidth + 200), height: Math.max(2000, contentHeight) }

  // Helper to get element center position
  const getPos = (el: HTMLElement) => {
    const rect = el.getBoundingClientRect()
    return {
      left: rect.left - containerRect.left + scrollLeft,
      right: rect.right - containerRect.left + scrollLeft,
      top: rect.top - containerRect.top + scrollTop,
      bottom: rect.bottom - containerRect.top + scrollTop,
      centerX: rect.left + rect.width / 2 - containerRect.left + scrollLeft,
      centerY: rect.top + rect.height / 2 - containerRect.top + scrollTop,
      width: rect.width,
      height: rect.height
    }
  }

  props.steps.forEach((step, stepIdx) => {
    // Transfer steps end the flow unless they have an explicit next_step
    if (step.message_type === 'transfer' && !step.next_step) {
      return // Skip drawing connections from transfer steps without explicit routing
    }

    // Branch steps: draw lines from true/false branch nodes
    if (isBranch(step)) {
      const { trueDest, falseDest } = getBranchDestinations(step, stepIdx)
      const branchPaths = [
        { key: 'true', dest: trueDest, label: 'True', color: '#22c55e' },
        { key: 'false', dest: falseDest, label: 'False', color: '#ef4444' }
      ]

      branchPaths.forEach((bp, bpIdx) => {
        const branchEl = buttonRefs.value.get(`branch-${stepIdx}-${bp.key}`)
        if (!branchEl) return

        const targetEl = bp.dest.targetIdx >= 0 ? stepRefs.value.get(bp.dest.targetIdx) : endRef.value
        if (!targetEl) return

        const from = getPos(branchEl)
        const to = getPos(targetEl)
        const isJump = bp.dest.targetIdx !== stepIdx + 1

        let path: string

        if (isJump && bp.dest.targetIdx >= 0) {
          const containerWidth = containerRef.value?.clientWidth || 800
          const centerX = containerWidth / 2
          const xOffset = 220 + (bpIdx * 35)

          if (bp.dest.targetIdx > stepIdx) {
            const rightX = centerX + xOffset
            path = `M ${from.centerX} ${from.bottom} ` +
                   `L ${from.centerX} ${from.bottom + 20} ` +
                   `L ${rightX} ${from.bottom + 20} ` +
                   `L ${rightX} ${to.centerY} ` +
                   `L ${to.right + 10} ${to.centerY}`
          } else {
            const leftX = centerX - xOffset
            path = `M ${from.centerX} ${from.bottom} ` +
                   `L ${from.centerX} ${from.bottom + 20} ` +
                   `L ${leftX} ${from.bottom + 20} ` +
                   `L ${leftX} ${to.centerY} ` +
                   `L ${to.left - 10} ${to.centerY}`
          }

          const verticalLineX = bp.dest.targetIdx > stepIdx ? centerX + xOffset : centerX - xOffset

          newConnections.push({
            path,
            color: bp.color,
            label: bp.label,
            labelX: verticalLineX,
            labelY: (from.bottom + 20 + to.centerY) / 2
          })
        } else {
          const xOff = (bpIdx - 0.5) * 15
          path = `M ${from.centerX} ${from.bottom} ` +
                 `L ${from.centerX} ${from.bottom + 15} ` +
                 `L ${to.centerX + xOff} ${to.top - 15} ` +
                 `L ${to.centerX + xOff} ${to.top}`

          newConnections.push({
            path,
            color: bp.color,
            label: bp.label,
            labelX: (from.centerX + to.centerX + xOff) / 2,
            labelY: (from.bottom + 15 + to.top - 15) / 2
          })
        }
      })
      return
    }

    if (!hasButtons(step)) {
      // Non-button step: check for explicit next_step jump or sequential flow
      const stepEl = stepRefs.value.get(stepIdx)
      if (!stepEl) return

      // Determine target: explicit next_step or next sequential step
      let targetIdx = -1
      let isExplicitJump = false
      let isExplicitEnd = false

      if (step.next_step) {
        if (step.next_step === '__end__') {
          // Explicit end flow
          isExplicitEnd = true
          isExplicitJump = true
        } else {
          // Explicit "go to" target
          const idx = props.steps.findIndex(s => s.step_name === step.next_step)
          if (idx >= 0) {
            targetIdx = idx
            isExplicitJump = targetIdx !== stepIdx + 1
          }
        }
      }

      if (targetIdx < 0 && !step.next_step) {
        // Sequential fallback (but not into branch targets)
        const nextStep = props.steps[stepIdx + 1]
        const nextIsBranchTarget = nextStep && isBranchTargetStep(nextStep.step_name)
        if (!nextIsBranchTarget && stepIdx + 1 < props.steps.length) {
          targetIdx = stepIdx + 1
        }
      }

      const targetEl = targetIdx >= 0 ? stepRefs.value.get(targetIdx) : endRef.value

      if (stepEl && targetEl) {
        const from = getPos(stepEl)
        const to = getPos(targetEl)

        if (isExplicitEnd) {
          // Explicit end flow - draw a direct line to END node
          const color = '#3b82f6'
          const path = `M ${from.centerX} ${from.bottom} L ${from.centerX} ${from.bottom + 20} L ${to.centerX} ${to.top - 20} L ${to.centerX} ${to.top}`
          newConnections.push({
            path,
            color,
            label: 'End',
            labelX: (from.centerX + to.centerX) / 2,
            labelY: (from.bottom + 20 + to.top - 20) / 2
          })
        } else if (isExplicitJump) {
          // Draw a jump line (similar to button jumps)
          const containerWidth = containerRef.value?.clientWidth || 800
          const centerX = containerWidth / 2
          const xOffset = 220
          const color = '#3b82f6' // blue for go-to jumps
          let path: string

          if (targetIdx > stepIdx) {
            const rightX = centerX + xOffset
            path = `M ${from.centerX} ${from.bottom} ` +
                   `L ${from.centerX} ${from.bottom + 20} ` +
                   `L ${rightX} ${from.bottom + 20} ` +
                   `L ${rightX} ${to.centerY} ` +
                   `L ${to.right + 10} ${to.centerY}`
          } else {
            const leftX = centerX - xOffset
            path = `M ${from.centerX} ${from.bottom} ` +
                   `L ${from.centerX} ${from.bottom + 20} ` +
                   `L ${leftX} ${from.bottom + 20} ` +
                   `L ${leftX} ${to.centerY} ` +
                   `L ${to.left - 10} ${to.centerY}`
          }

          const verticalLineX = targetIdx > stepIdx ? centerX + xOffset : centerX - xOffset
          newConnections.push({
            path,
            color,
            label: step.next_step,
            labelX: verticalLineX,
            labelY: (from.bottom + 20 + to.centerY) / 2
          })
        } else {
          // Simple straight line down (sequential or to END)
          newConnections.push({
            path: `M ${from.centerX} ${from.bottom} L ${from.centerX} ${from.bottom + 20} L ${to.centerX} ${to.top - 20} L ${to.centerX} ${to.top}`,
            color: '#9ca3af',
            label: '',
            labelX: 0,
            labelY: 0
          })
        }
      }
    } else {
      // Button step: draw lines from each button to target
      const replyButtons = getReplyButtons(step)
      const stepEl = stepRefs.value.get(stepIdx)

      // Track how many lines go to each target (for offsetting)
      const targetCounts: Record<number, number> = {}

      replyButtons.forEach((btn, btnIdx) => {
        const buttonEl = buttonRefs.value.get(`${stepIdx}-${btnIdx}`)
        if (!buttonEl || !stepEl) return

        const dest = getButtonDestination(step, stepIdx, btn, btnIdx)
        const targetEl = dest.targetIdx >= 0 ? stepRefs.value.get(dest.targetIdx) : endRef.value

        if (!targetEl) return

        const from = getPos(buttonEl)
        const to = getPos(targetEl)

        // Determine if it's a jump (non-sequential)
        const isJump = dest.targetIdx !== stepIdx + 1

        let path: string
        const color = lineColors[colorIdx % lineColors.length]

        if (isJump && dest.targetIdx === -1) {
          // Explicit end flow - draw colored line to END node
          colorIdx++
          const path = `M ${from.centerX} ${from.bottom} ` +
                 `L ${from.centerX} ${from.bottom + 15} ` +
                 `L ${to.centerX} ${to.top - 15} ` +
                 `L ${to.centerX} ${to.top}`

          newConnections.push({
            path,
            color,
            label: btn.title ? `${btn.title} → End` : 'End',
            labelX: (from.centerX + to.centerX) / 2,
            labelY: (from.bottom + 15 + to.top - 15) / 2
          })
        } else if (isJump && dest.targetIdx >= 0) {
          // Track this target for Y offset
          if (!targetCounts[dest.targetIdx]) targetCounts[dest.targetIdx] = 0
          const targetCount = targetCounts[dest.targetIdx]
          targetCounts[dest.targetIdx]++

          // Curved line around cards for jumps
          const containerWidth = containerRef.value?.clientWidth || 800
          const centerX = containerWidth / 2

          // Calculate X offset - each button gets different offset
          const baseOffset = 220
          const offsetIncrement = 35
          const xOffset = baseOffset + (btnIdx * offsetIncrement)

          // Calculate Y offset for entry point - 40px apart (larger than label height 28px)
          const yEntryOffset = (targetCount - 0.5) * 40

          if (dest.targetIdx > stepIdx) {
            // Forward jump - curve to the RIGHT side
            const rightX = centerX + xOffset
            const entryY = to.centerY + yEntryOffset
            path = `M ${from.centerX} ${from.bottom} ` +
                   `L ${from.centerX} ${from.bottom + 20} ` +
                   `L ${rightX} ${from.bottom + 20} ` +
                   `L ${rightX} ${entryY} ` +
                   `L ${to.right + 10} ${entryY}`
          } else {
            // Backward jump - curve to the LEFT side
            const leftX = centerX - xOffset
            const entryY = to.centerY + yEntryOffset
            path = `M ${from.centerX} ${from.bottom} ` +
                   `L ${from.centerX} ${from.bottom + 20} ` +
                   `L ${leftX} ${from.bottom + 20} ` +
                   `L ${leftX} ${entryY} ` +
                   `L ${to.left - 10} ${entryY}`
          }
          colorIdx++

          // Position label on the VERTICAL portion of the line (not near cards)
          const entryY = to.centerY + yEntryOffset
          const verticalLineX = dest.targetIdx > stepIdx ? centerX + xOffset : centerX - xOffset

          // Label positioned on vertical line, midway between start horizontal and entry point
          const labelX = verticalLineX
          const labelY = (from.bottom + 20 + entryY) / 2

          newConnections.push({
            path,
            color,
            label: btn.title || `Btn ${btnIdx + 1}`,
            labelX: labelX,
            labelY: labelY
          })
        } else {
          // Sequential flow - straight down with slight offset for each button
          const xOff = (btnIdx - (replyButtons.length - 1) / 2) * 15
          path = `M ${from.centerX} ${from.bottom} ` +
                 `L ${from.centerX} ${from.bottom + 15} ` +
                 `L ${to.centerX + xOff} ${to.top - 15} ` +
                 `L ${to.centerX + xOff} ${to.top}`

          newConnections.push({
            path,
            color: '#9ca3af',
            label: '',
            labelX: 0,
            labelY: 0
          })
        }
      })
    }
  })

  // Start to first step
  if (startRef.value && stepRefs.value.get(0)) {
    const from = getPos(startRef.value)
    const to = getPos(stepRefs.value.get(0)!)

    // Start from bottom of START div (below text), end above first step
    newConnections.unshift({
      path: `M ${from.centerX} ${from.bottom} L ${from.centerX} ${to.top - 15}`,
      color: '#22c55e',
      label: '',
      labelX: 0,
      labelY: 0
    })
  }

  connections.value = newConnections
}

// Debounced update
let updateTimeout: number | null = null
function debouncedUpdate() {
  if (updateTimeout) clearTimeout(updateTimeout)
  updateTimeout = window.setTimeout(updateConnections, 50)
}

watch(() => [props.steps, props.selectedStepIndex], () => {
  nextTick(debouncedUpdate)
}, { deep: true })

onMounted(() => {
  nextTick(() => setTimeout(updateConnections, 100))
  window.addEventListener('resize', debouncedUpdate)
})

onUnmounted(() => {
  window.removeEventListener('resize', debouncedUpdate)
  if (updateTimeout) clearTimeout(updateTimeout)
})
</script>

<template>
  <div class="h-full flex flex-col bg-[#fafafa] dark:bg-[#0a0a0a] overflow-hidden">
    <!-- Header -->
    <div class="px-4 py-3 border-b bg-white dark:bg-[#111] flex items-center justify-between flex-shrink-0">
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

    <!-- Flowchart Canvas -->
    <div
      ref="containerRef"
      class="flex-1 overflow-auto relative"
      @scroll="debouncedUpdate"
    >
      <!-- SVG Connection Lines - Now with higher z-index -->
      <svg
        class="absolute top-0 left-0 pointer-events-none"
        style="z-index: 5;"
        :width="svgSize.width"
        :height="svgSize.height"
      >
        <defs>
          <marker
            id="arrow-gray"
            markerWidth="12"
            markerHeight="10"
            refX="10"
            refY="5"
            orient="auto"
          >
            <path d="M 0 0 L 12 5 L 0 10 L 3 5 Z" fill="#9ca3af" />
          </marker>
          <marker
            v-for="(color, idx) in lineColors"
            :key="idx"
            :id="`arrow-${idx}`"
            markerWidth="12"
            markerHeight="10"
            refX="10"
            refY="5"
            orient="auto"
          >
            <path d="M 0 0 L 12 5 L 0 10 L 3 5 Z" :fill="color" />
          </marker>
          <marker
            id="arrow-green"
            markerWidth="12"
            markerHeight="10"
            refX="10"
            refY="5"
            orient="auto"
          >
            <path d="M 0 0 L 12 5 L 0 10 L 3 5 Z" fill="#22c55e" />
          </marker>
          <marker
            id="arrow-red"
            markerWidth="12"
            markerHeight="10"
            refX="10"
            refY="5"
            orient="auto"
          >
            <path d="M 0 0 L 12 5 L 0 10 L 3 5 Z" fill="#ef4444" />
          </marker>
        </defs>

        <g v-for="(conn, idx) in connections" :key="idx">
          <!-- Shadow for better visibility (only for colored jump lines, not gray, green, or red) -->
          <path
            v-if="conn.color !== '#9ca3af' && conn.color !== '#22c55e' && conn.color !== '#ef4444'"
            :d="conn.path"
            fill="none"
            stroke="white"
            stroke-width="6"
            stroke-linecap="round"
            stroke-linejoin="round"
          />
          <path
            :d="conn.path"
            fill="none"
            :stroke="conn.color"
            :stroke-width="conn.color === '#9ca3af' ? 2 : 3"
            stroke-linecap="round"
            stroke-linejoin="round"
            :marker-end="conn.color === '#22c55e' ? 'url(#arrow-green)' : conn.color === '#ef4444' ? 'url(#arrow-red)' : conn.color === '#9ca3af' ? 'url(#arrow-gray)' : `url(#arrow-${lineColors.indexOf(conn.color)})`"
          />
          <!-- Label for jump connections - positioned ON the line with solid background -->
          <g v-if="conn.label">
            <!-- White background for visibility -->
            <rect
              :x="conn.labelX - 50"
              :y="conn.labelY - 14"
              width="100"
              height="28"
              rx="14"
              fill="white"
              stroke="#e5e7eb"
              stroke-width="2"
              filter="drop-shadow(0 2px 4px rgba(0,0,0,0.1))"
            />
            <!-- Colored inner background -->
            <rect
              :x="conn.labelX - 47"
              :y="conn.labelY - 11"
              width="94"
              height="22"
              rx="11"
              :fill="conn.color"
            />
            <!-- Label text -->
            <text
              :x="conn.labelX"
              :y="conn.labelY + 5"
              text-anchor="middle"
              fill="white"
              font-size="11"
              font-weight="700"
              style="text-shadow: 0 1px 2px rgba(0,0,0,0.3)"
            >
              {{ conn.label.length > 12 ? conn.label.substring(0, 12) + '…' : conn.label }}
            </text>
          </g>
        </g>
      </svg>

      <!-- Flow Nodes -->
      <div class="relative p-8 flex flex-col items-center min-h-full" style="z-index: 1;">
        <!-- Start Node -->
        <div
          ref="startRef"
          class="flex flex-col items-center cursor-pointer group mb-16"
          @click="emit('selectFlowSettings')"
        >
          <span class="mb-2 text-sm font-bold text-green-600">START</span>
          <div class="w-16 h-16 rounded-full bg-green-500 flex items-center justify-center shadow-lg group-hover:ring-4 group-hover:ring-green-200 transition-all">
            <Play class="h-7 w-7 text-white" />
          </div>
        </div>

        <!-- Steps -->
        <template v-for="(step, idx) in steps" :key="`step-${idx}`">
          <!-- Step Card -->
          <div
            :ref="(el) => setStepRef(el, idx)"
            :class="[
              'w-80 rounded-xl border-2 shadow-lg cursor-pointer transition-all mb-4 relative',
              isUnreachable(idx)
                ? 'border-red-400 bg-red-50 dark:bg-red-900/20 opacity-60'
                : isInLoop(idx)
                  ? 'border-amber-400 bg-amber-50 dark:bg-amber-900/20'
                  : selectedStepIndex === idx
                    ? 'border-primary bg-white dark:bg-[#1a1a1a] ring-4 ring-primary/20'
                    : 'border-gray-200 dark:border-gray-700 bg-white dark:bg-[#1a1a1a] hover:border-gray-300 hover:shadow-xl'
            ]"
            @click="emit('selectStep', idx)"
          >
            <!-- Loop Warning Badge -->
            <div
              v-if="isInLoop(idx) && !isUnreachable(idx)"
              class="absolute -top-3 left-1/2 -translate-x-1/2 bg-amber-500 text-white text-[10px] font-bold px-3 py-1 rounded-full flex items-center gap-1 shadow-md z-10"
            >
              <svg class="h-3 w-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M21 12a9 9 0 1 1-6.219-8.56" />
              </svg>
              LOOP
            </div>
            <!-- Unreachable Warning Badge -->
            <div
              v-if="isUnreachable(idx)"
              class="absolute -top-3 left-1/2 -translate-x-1/2 bg-red-500 text-white text-[10px] font-bold px-3 py-1 rounded-full flex items-center gap-1 shadow-md z-10"
            >
              <AlertTriangle class="h-3 w-3" />
              UNREACHABLE
            </div>

            <!-- Header -->
            <div :class="['flex items-center gap-3 p-3 rounded-t-xl', isUnreachable(idx) ? 'bg-red-100 dark:bg-red-900/30' : getStepColor(step.message_type) + ' bg-opacity-10 dark:bg-opacity-20']">
              <div :class="['w-9 h-9 rounded-lg flex items-center justify-center', isUnreachable(idx) ? 'bg-red-400' : getStepColor(step.message_type)]">
                <component :is="isUnreachable(idx) ? AlertTriangle : getStepIcon(step.message_type)" class="h-4 w-4 text-white" />
              </div>
              <span class="font-semibold text-sm flex-1 truncate">{{ step.step_name || `Step ${idx + 1}` }}</span>
              <Badge :variant="isUnreachable(idx) ? 'destructive' : 'secondary'" class="text-xs h-6 px-2">{{ idx + 1 }}</Badge>
            </div>
            <!-- Message -->
            <div class="p-3">
              <p class="text-xs text-muted-foreground line-clamp-2">{{ step.message || 'No message' }}</p>
            </div>
            <!-- Branch indicator -->
            <div v-if="isBranch(step)" class="px-3 pb-3">
              <div class="text-[10px] text-muted-foreground flex items-center gap-1">
                <GitMerge class="h-3 w-3" />
                <span>Branch on: {{ step.conditional_next ? 'configured' : 'not configured' }}</span>
              </div>
            </div>
            <!-- Buttons indicator -->
            <div v-if="hasButtons(step)" class="px-3 pb-3">
              <div class="text-[10px] text-muted-foreground flex items-center gap-1">
                <GitBranch class="h-3 w-3" />
                <span>{{ getReplyButtons(step).length }} button{{ getReplyButtons(step).length > 1 ? 's' : '' }} - routes to specific steps</span>
              </div>
            </div>
          </div>

          <!-- Branch nodes (shown below step if branch) -->
          <div v-if="isBranch(step)" class="flex gap-3 mb-8 justify-center">
            <div
              :ref="(el) => { if (el) buttonRefs.set(`branch-${idx}-true`, el as HTMLElement) }"
              :class="[
                'px-4 py-2.5 rounded-xl border-2 text-sm font-medium shadow-md cursor-pointer transition-all min-w-[120px]',
                getBranchDestinations(step, idx).trueDest.targetIdx !== idx + 1
                  ? 'bg-green-50 dark:bg-green-900/30 border-green-400 text-green-700 dark:text-green-300 hover:bg-green-100'
                  : 'bg-white dark:bg-gray-800 border-green-300 dark:border-green-600 text-green-700 dark:text-green-300 hover:bg-green-50'
              ]"
              @click.stop="emit('selectStep', idx)"
            >
              <div class="text-center">
                <div class="font-semibold">True</div>
                <div class="text-[10px] mt-1 text-green-500">
                  → {{ getBranchDestinations(step, idx).trueDest.targetName }}
                </div>
              </div>
            </div>
            <div
              :ref="(el) => { if (el) buttonRefs.set(`branch-${idx}-false`, el as HTMLElement) }"
              :class="[
                'px-4 py-2.5 rounded-xl border-2 text-sm font-medium shadow-md cursor-pointer transition-all min-w-[120px]',
                getBranchDestinations(step, idx).falseDest.targetIdx !== idx + 1
                  ? 'bg-red-50 dark:bg-red-900/30 border-red-400 text-red-700 dark:text-red-300 hover:bg-red-100'
                  : 'bg-white dark:bg-gray-800 border-red-300 dark:border-red-600 text-red-700 dark:text-red-300 hover:bg-red-50'
              ]"
              @click.stop="emit('selectStep', idx)"
            >
              <div class="text-center">
                <div class="font-semibold">False</div>
                <div class="text-[10px] mt-1 text-red-500">
                  → {{ getBranchDestinations(step, idx).falseDest.targetName }}
                </div>
              </div>
            </div>
          </div>

          <!-- Button nodes (shown below step if has buttons) -->
          <div v-if="hasButtons(step)" class="flex gap-3 mb-8 flex-wrap justify-center">
            <div
              v-for="(btn, btnIdx) in getReplyButtons(step)"
              :key="btnIdx"
              :ref="(el) => setButtonRef(el, idx, btnIdx)"
              :class="[
                'px-4 py-2.5 rounded-xl border-2 text-sm font-medium shadow-md cursor-pointer transition-all min-w-[120px]',
                getButtonDestination(step, idx, btn, btnIdx).targetIdx !== idx + 1
                  ? 'bg-purple-50 dark:bg-purple-900/30 border-purple-400 text-purple-700 dark:text-purple-300 hover:bg-purple-100'
                  : 'bg-white dark:bg-gray-800 border-gray-300 dark:border-gray-600 text-gray-700 dark:text-gray-300 hover:bg-gray-50'
              ]"
              @click.stop="emit('selectStep', idx)"
            >
              <div class="text-center">
                <div class="font-semibold">{{ btn.title || `Button ${btnIdx + 1}` }}</div>
                <div :class="['text-[10px] mt-1', getButtonDestination(step, idx, btn, btnIdx).targetIdx !== idx + 1 ? 'text-purple-500' : 'text-gray-400']">
                  → {{ getButtonDestination(step, idx, btn, btnIdx).targetName }}
                </div>
              </div>
            </div>
          </div>

          <!-- Spacer for non-button, non-branch steps -->
          <div v-else-if="!isBranch(step)" class="h-8"></div>
        </template>

        <!-- End Node -->
        <div
          ref="endRef"
          class="flex flex-col items-center cursor-pointer group mt-4 relative"
          @click="emit('selectFlowSettings')"
        >
          <!-- Completely Unreachable Warning -->
          <div
            v-if="!isEndReachable && steps.length > 0"
            class="absolute -top-3 left-1/2 -translate-x-1/2 bg-red-500 text-white text-[10px] font-bold px-3 py-1 rounded-full flex items-center gap-1 shadow-md z-10 whitespace-nowrap"
          >
            <svg class="h-3 w-3 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M21 12a9 9 0 1 1-6.219-8.56" />
            </svg>
            UNREACHABLE
          </div>
          <!-- Partial Loop Warning (END reachable but some paths loop) -->
          <div
            v-else-if="stepsInLoop.size > 0 && steps.length > 0"
            class="absolute -top-3 left-1/2 -translate-x-1/2 bg-amber-500 text-white text-[10px] font-bold px-3 py-1 rounded-full flex items-center gap-1 shadow-md z-10 whitespace-nowrap"
          >
            <svg class="h-3 w-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M21 12a9 9 0 1 1-6.219-8.56" />
            </svg>
            SOME PATHS LOOP
          </div>
          <div
            :class="[
              'w-16 h-16 rounded-full flex items-center justify-center shadow-lg transition-all',
              !isEndReachable && steps.length > 0
                ? 'bg-gray-400 opacity-50 group-hover:ring-4 group-hover:ring-gray-200'
                : stepsInLoop.size > 0 && steps.length > 0
                  ? 'bg-amber-500 group-hover:ring-4 group-hover:ring-amber-200'
                  : 'bg-red-500 group-hover:ring-4 group-hover:ring-red-200'
            ]"
          >
            <Flag class="h-7 w-7 text-white" />
          </div>
          <span :class="[
            'mt-2 text-sm font-bold',
            !isEndReachable && steps.length > 0
              ? 'text-gray-400'
              : stepsInLoop.size > 0 && steps.length > 0
                ? 'text-amber-600'
                : 'text-red-600'
          ]">
            {{ !isEndReachable && steps.length > 0 ? 'UNREACHABLE' : 'END' }}
          </span>
          <p v-if="!isEndReachable && steps.length > 0" class="text-xs text-red-600 mt-1 text-center max-w-[200px]">
            Flow has no exit path - will loop forever
          </p>
          <p v-else-if="stepsInLoop.size > 0 && steps.length > 0" class="text-xs text-amber-600 mt-1 text-center max-w-[200px]">
            Some paths never reach END
          </p>
        </div>

        <!-- Empty State -->
        <template v-if="steps.length === 0">
          <div
            class="w-72 py-12 rounded-xl border-2 border-dashed border-gray-300 dark:border-gray-600 flex flex-col items-center justify-center cursor-pointer hover:border-primary hover:bg-primary/5 transition-all my-8"
            @click="emit('addStep')"
          >
            <Plus class="h-10 w-10 text-gray-400 mb-3" />
            <span class="text-sm font-medium text-muted-foreground">Add your first step</span>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>
