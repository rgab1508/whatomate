<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { chatbotService } from '@/services/api'
import { PageHeader } from '@/components/shared'
import {
  Activity, ArrowLeft, Play, LogIn, MessageSquare, CheckCircle, XCircle,
  SkipForward, GitBranch, Globe, ArrowRightLeft, Flag, Ban, AlertCircle,
  Clock, User, Phone
} from 'lucide-vue-next'

const route = useRoute()
const flowId = route.params.id as string
const sessionId = route.params.runId as string

interface FlowLog {
  id: string
  session_id: string
  flow_id: string
  step_name: string
  event_type: string
  message: string
  data: Record<string, any>
  created_at: string
}

interface SessionInfo {
  id: string
  phone_number: string
  status: string
  current_step: string
  started_at: string
  completed_at?: string
  last_activity_at: string
  session_data: Record<string, any>
  contact?: { id: string; profile_name: string; phone_number: string }
}

const logs = ref<FlowLog[]>([])
const session = ref<SessionInfo | null>(null)
const isLoading = ref(true)

onMounted(async () => {
  await fetchLogs()
})

async function fetchLogs() {
  isLoading.value = true
  try {
    const response = await chatbotService.getSessionLogs(sessionId)
    const data = (response.data as any).data || response.data
    logs.value = data.logs || []
    session.value = data.session || null
  } catch (error) {
    console.error('Failed to load run logs:', error)
  } finally {
    isLoading.value = false
  }
}

function getEventIcon(eventType: string) {
  switch (eventType) {
    case 'flow_started': return Play
    case 'step_sent': return LogIn
    case 'input_received': return MessageSquare
    case 'input_validated': return CheckCircle
    case 'input_invalid': return XCircle
    case 'step_skipped': return SkipForward
    case 'branch_evaluated': return GitBranch
    case 'api_called': return Globe
    case 'transfer_initiated': return ArrowRightLeft
    case 'flow_completed': return Flag
    case 'flow_cancelled': return Ban
    case 'error': return AlertCircle
    default: return Activity
  }
}

function getEventColor(eventType: string) {
  switch (eventType) {
    case 'flow_started': return 'text-green-500'
    case 'step_sent': return 'text-blue-500'
    case 'input_received': return 'text-cyan-500'
    case 'input_validated': return 'text-green-400'
    case 'input_invalid': return 'text-red-500'
    case 'step_skipped': return 'text-yellow-500'
    case 'branch_evaluated': return 'text-amber-500'
    case 'api_called': return 'text-purple-500'
    case 'transfer_initiated': return 'text-orange-500'
    case 'flow_completed': return 'text-green-600'
    case 'flow_cancelled': return 'text-red-400'
    case 'error': return 'text-red-600'
    default: return 'text-gray-500'
  }
}

function getEventBgColor(eventType: string) {
  switch (eventType) {
    case 'flow_started': return 'bg-green-500/10 border-green-500/20'
    case 'step_sent': return 'bg-blue-500/10 border-blue-500/20'
    case 'input_received': return 'bg-cyan-500/10 border-cyan-500/20'
    case 'input_validated': return 'bg-green-400/10 border-green-400/20'
    case 'input_invalid': return 'bg-red-500/10 border-red-500/20'
    case 'step_skipped': return 'bg-yellow-500/10 border-yellow-500/20'
    case 'branch_evaluated': return 'bg-amber-500/10 border-amber-500/20'
    case 'api_called': return 'bg-purple-500/10 border-purple-500/20'
    case 'transfer_initiated': return 'bg-orange-500/10 border-orange-500/20'
    case 'flow_completed': return 'bg-green-600/10 border-green-600/20'
    case 'flow_cancelled': return 'bg-red-400/10 border-red-400/20'
    case 'error': return 'bg-red-600/10 border-red-600/20'
    default: return 'bg-gray-500/10 border-gray-500/20'
  }
}

function formatTime(dateStr: string) {
  if (!dateStr) return ''
  return new Date(dateStr).toLocaleTimeString('en-US', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false
  })
}

function formatDate(dateStr: string) {
  if (!dateStr) return '—'
  return new Date(dateStr).toLocaleString()
}

function formatDuration() {
  if (!session.value?.completed_at) return null
  const start = new Date(session.value.started_at).getTime()
  const end = new Date(session.value.completed_at).getTime()
  const diff = end - start
  if (diff < 1000) return '<1s'
  if (diff < 60000) return `${Math.round(diff / 1000)}s`
  if (diff < 3600000) {
    const mins = Math.floor(diff / 60000)
    const secs = Math.round((diff % 60000) / 1000)
    return `${mins}m ${secs}s`
  }
  return `${Math.round(diff / 3600000)}h`
}

function statusColor(status: string) {
  switch (status) {
    case 'completed': return 'bg-green-500/10 text-green-500 border-green-500/20'
    case 'active': return 'bg-blue-500/10 text-blue-500 border-blue-500/20'
    case 'cancelled': return 'bg-red-500/10 text-red-500 border-red-500/20'
    case 'timeout': return 'bg-yellow-500/10 text-yellow-500 border-yellow-500/20'
    default: return ''
  }
}

function formatDataValue(value: any): string {
  if (value === null || value === undefined) return '—'
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

const relevantDataKeys = ['user_input', 'button_id', 'store_as', 'value', 'variable', 'operator',
  'var_value', 'result', 'error', 'skip_condition', 'validation_regex', 'retry',
  'team_id', 'notes', 'cancel_keyword', 'flow_name', 'num_steps', 'message_type', 'input_type']

function getRelevantData(data: Record<string, any>) {
  if (!data) return []
  return Object.entries(data)
    .filter(([key]) => relevantDataKeys.includes(key))
    .filter(([_, value]) => value !== '' && value !== null && value !== undefined)
}
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader
      title="Run Logs"
      :icon="Activity"
      icon-gradient="bg-gradient-to-br from-indigo-500 to-purple-600 shadow-indigo-500/20"
      :back-link="`/chatbot/flows/${flowId}/runs`"
      :breadcrumbs="[
        { label: 'Chatbot', href: '/chatbot' },
        { label: 'Flows', href: '/chatbot/flows' },
        { label: 'Runs', href: `/chatbot/flows/${flowId}/runs` },
        { label: 'Logs' }
      ]"
    >
      <template #actions>
        <Button variant="outline" size="sm" @click="$router.push(`/chatbot/flows/${flowId}/runs`)">
          <ArrowLeft class="h-4 w-4 mr-2" />
          Back to Runs
        </Button>
      </template>
    </PageHeader>

    <ScrollArea class="flex-1">
      <div class="p-6">
        <div class="max-w-4xl mx-auto space-y-6">

          <!-- Session Info Card -->
          <Card v-if="session">
            <CardHeader class="pb-3">
              <CardTitle class="text-lg">Session Info</CardTitle>
            </CardHeader>
            <CardContent>
              <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
                <div class="space-y-1">
                  <div class="text-xs text-muted-foreground flex items-center gap-1">
                    <User class="h-3 w-3" /> Contact
                  </div>
                  <div class="text-sm font-medium">{{ session.contact?.profile_name || 'Unknown' }}</div>
                </div>
                <div class="space-y-1">
                  <div class="text-xs text-muted-foreground flex items-center gap-1">
                    <Phone class="h-3 w-3" /> Phone
                  </div>
                  <div class="text-sm font-medium">{{ session.phone_number }}</div>
                </div>
                <div class="space-y-1">
                  <div class="text-xs text-muted-foreground flex items-center gap-1">
                    <Activity class="h-3 w-3" /> Status
                  </div>
                  <Badge :class="statusColor(session.status)">{{ session.status }}</Badge>
                </div>
                <div class="space-y-1">
                  <div class="text-xs text-muted-foreground flex items-center gap-1">
                    <Clock class="h-3 w-3" /> Duration
                  </div>
                  <div class="text-sm font-medium">{{ formatDuration() || 'In progress' }}</div>
                </div>
              </div>
              <div class="grid grid-cols-2 gap-4 mt-4 pt-4 border-t border-white/[0.08] light:border-gray-200">
                <div class="space-y-1">
                  <div class="text-xs text-muted-foreground">Started</div>
                  <div class="text-sm">{{ formatDate(session.started_at) }}</div>
                </div>
                <div class="space-y-1">
                  <div class="text-xs text-muted-foreground">Completed</div>
                  <div class="text-sm">{{ session.completed_at ? formatDate(session.completed_at) : '—' }}</div>
                </div>
              </div>
            </CardContent>
          </Card>

          <!-- Session Data Card -->
          <Card v-if="session?.session_data && Object.keys(session.session_data).filter(k => !k.startsWith('_')).length > 0">
            <CardHeader class="pb-3">
              <CardTitle class="text-lg">Collected Data</CardTitle>
              <CardDescription>Variables collected during this flow run.</CardDescription>
            </CardHeader>
            <CardContent>
              <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                <div
                  v-for="[key, value] in Object.entries(session.session_data).filter(([k]) => !k.startsWith('_'))"
                  :key="key"
                  class="flex items-start gap-2 p-2 rounded-lg bg-white/[0.03] light:bg-gray-50 border border-white/[0.06] light:border-gray-200"
                >
                  <code class="text-xs text-purple-400 light:text-purple-600 font-mono shrink-0">{{ key }}</code>
                  <span class="text-xs text-muted-foreground break-all">{{ formatDataValue(value) }}</span>
                </div>
              </div>
            </CardContent>
          </Card>

          <!-- Execution Timeline -->
          <Card>
            <CardHeader class="pb-3">
              <div class="flex items-center justify-between">
                <div>
                  <CardTitle class="text-lg">Execution Timeline</CardTitle>
                  <CardDescription>Step-by-step trace of what happened during this flow run.</CardDescription>
                </div>
                <Badge variant="outline" class="text-xs">{{ logs.length }} events</Badge>
              </div>
            </CardHeader>
            <CardContent>
              <!-- Loading state -->
              <div v-if="isLoading" class="space-y-3">
                <div v-for="i in 5" :key="i" class="h-16 rounded-lg bg-white/[0.03] light:bg-gray-100 animate-pulse" />
              </div>

              <!-- Empty state -->
              <div v-else-if="logs.length === 0" class="text-center py-12">
                <Activity class="h-10 w-10 mx-auto text-muted-foreground mb-3" />
                <p class="text-muted-foreground">No execution logs recorded for this session.</p>
              </div>

              <!-- Timeline -->
              <div v-else class="relative">
                <!-- Timeline line -->
                <div class="absolute left-[19px] top-4 bottom-4 w-px bg-white/[0.08] light:bg-gray-200" />

                <div v-for="log in logs" :key="log.id" class="relative flex gap-4 pb-4 last:pb-0">
                  <!-- Timeline dot -->
                  <div
                    class="relative z-10 flex items-center justify-center w-10 h-10 rounded-full border shrink-0"
                    :class="getEventBgColor(log.event_type)"
                  >
                    <component
                      :is="getEventIcon(log.event_type)"
                      class="h-4 w-4"
                      :class="getEventColor(log.event_type)"
                    />
                  </div>

                  <!-- Content -->
                  <div class="flex-1 min-w-0 pt-1">
                    <div class="flex items-center gap-2 flex-wrap">
                      <span class="text-sm font-medium">{{ log.message }}</span>
                      <Badge v-if="log.step_name" variant="outline" class="text-xs font-mono">
                        {{ log.step_name }}
                      </Badge>
                    </div>

                    <!-- Event data -->
                    <div v-if="getRelevantData(log.data).length > 0" class="mt-1.5 flex flex-wrap gap-2">
                      <div
                        v-for="[key, value] in getRelevantData(log.data)"
                        :key="key"
                        class="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-md bg-white/[0.04] light:bg-gray-100 border border-white/[0.06] light:border-gray-200"
                      >
                        <span class="text-muted-foreground">{{ key }}:</span>
                        <span class="font-mono max-w-[200px] truncate">{{ formatDataValue(value) }}</span>
                      </div>
                    </div>

                    <div class="text-xs text-muted-foreground mt-1">
                      {{ formatTime(log.created_at) }}
                    </div>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>

        </div>
      </div>
    </ScrollArea>
  </div>
</template>
