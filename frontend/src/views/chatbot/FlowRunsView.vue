<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { chatbotService } from '@/services/api'
import { PageHeader, DataTable, type Column } from '@/components/shared'
import { Activity, ArrowLeft, Eye } from 'lucide-vue-next'

const route = useRoute()
const router = useRouter()
const flowId = route.params.id as string

interface FlowRun {
  id: string
  contact?: { id: string; profile_name: string; phone_number: string }
  phone_number: string
  status: string
  current_step: string
  started_at: string
  completed_at?: string
  last_activity_at: string
  session_data: Record<string, any>
}

interface FlowInfo {
  id: string
  name: string
  description: string
}

const runs = ref<FlowRun[]>([])
const flow = ref<FlowInfo | null>(null)
const isLoading = ref(true)
const statusFilter = ref('all')

const columns = computed<Column<FlowRun>[]>(() => [
  { key: 'contact', label: 'Contact' },
  { key: 'status', label: 'Status', sortable: true },
  { key: 'current_step', label: 'Last Step' },
  { key: 'started_at', label: 'Started', sortable: true },
  { key: 'duration', label: 'Duration' },
  { key: 'actions', label: '', align: 'right' as const },
])

onMounted(async () => {
  await fetchRuns()
})

async function fetchRuns() {
  isLoading.value = true
  try {
    const params: any = {}
    if (statusFilter.value !== 'all') {
      params.status = statusFilter.value
    }
    const response = await chatbotService.listFlowRuns(flowId, params)
    const data = (response.data as any).data || response.data
    runs.value = data.runs || []
    flow.value = data.flow || null
  } catch (error) {
    console.error('Failed to load flow runs:', error)
    runs.value = []
  } finally {
    isLoading.value = false
  }
}

function viewRunLogs(run: FlowRun) {
  router.push(`/chatbot/flows/${flowId}/runs/${run.id}`)
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

function formatDate(dateStr: string) {
  if (!dateStr) return '—'
  return new Date(dateStr).toLocaleString()
}

function formatDuration(run: FlowRun) {
  if (!run.completed_at) {
    if (run.status === 'active') return 'In progress'
    return '—'
  }
  const start = new Date(run.started_at).getTime()
  const end = new Date(run.completed_at).getTime()
  const diff = end - start
  if (diff < 1000) return '<1s'
  if (diff < 60000) return `${Math.round(diff / 1000)}s`
  if (diff < 3600000) return `${Math.round(diff / 60000)}m`
  return `${Math.round(diff / 3600000)}h`
}

function onStatusChange(value: any) {
  statusFilter.value = String(value ?? 'all')
  fetchRuns()
}
</script>

<template>
  <div class="flex flex-col h-full bg-[#0a0a0b] light:bg-gray-50">
    <PageHeader
      :title="flow ? `${flow.name} — Runs` : 'Flow Runs'"
      :icon="Activity"
      icon-gradient="bg-gradient-to-br from-indigo-500 to-purple-600 shadow-indigo-500/20"
      :back-link="`/chatbot/flows/${flowId}/edit`"
      :breadcrumbs="[
        { label: 'Chatbot', href: '/chatbot' },
        { label: 'Flows', href: '/chatbot/flows' },
        { label: flow?.name || '...', href: `/chatbot/flows/${flowId}/edit` },
        { label: 'Runs' }
      ]"
    >
      <template #actions>
        <Button variant="outline" size="sm" @click="$router.push(`/chatbot/flows/${flowId}/edit`)">
          <ArrowLeft class="h-4 w-4 mr-2" />
          Back to Flow
        </Button>
      </template>
    </PageHeader>

    <ScrollArea class="flex-1">
      <div class="p-6">
        <div class="max-w-6xl mx-auto">
          <Card>
            <CardHeader>
              <div class="flex items-center justify-between flex-wrap gap-4">
                <div>
                  <CardTitle>Run History</CardTitle>
                  <CardDescription>View each session that ran through this flow and inspect what happened step by step.</CardDescription>
                </div>
                <Select :model-value="statusFilter" @update:model-value="onStatusChange">
                  <SelectTrigger class="w-[160px]">
                    <SelectValue placeholder="All statuses" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">All statuses</SelectItem>
                    <SelectItem value="completed">Completed</SelectItem>
                    <SelectItem value="active">Active</SelectItem>
                    <SelectItem value="cancelled">Cancelled</SelectItem>
                    <SelectItem value="timeout">Timeout</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </CardHeader>
            <CardContent>
              <DataTable
                :items="runs"
                :columns="columns"
                :is-loading="isLoading"
                :empty-icon="Activity"
                empty-title="No runs yet"
                empty-description="This flow hasn't been triggered yet. Runs will appear here once contacts interact with this flow."
                item-name="runs"
              >
                <template #cell-contact="{ item: run }">
                  <div>
                    <div class="font-medium">{{ run.contact?.profile_name || 'Unknown' }}</div>
                    <div class="text-xs text-muted-foreground">{{ run.phone_number }}</div>
                  </div>
                </template>
                <template #cell-status="{ item: run }">
                  <Badge :class="statusColor(run.status)">
                    {{ run.status }}
                  </Badge>
                </template>
                <template #cell-current_step="{ item: run }">
                  <span class="text-muted-foreground text-sm">{{ run.current_step || '—' }}</span>
                </template>
                <template #cell-started_at="{ item: run }">
                  <span class="text-sm text-muted-foreground">{{ formatDate(run.started_at) }}</span>
                </template>
                <template #cell-duration="{ item: run }">
                  <span class="text-sm text-muted-foreground">{{ formatDuration(run) }}</span>
                </template>
                <template #cell-actions="{ item: run }">
                  <Button variant="ghost" size="sm" @click="viewRunLogs(run)">
                    <Eye class="h-4 w-4 mr-1" />
                    View Logs
                  </Button>
                </template>
              </DataTable>
            </CardContent>
          </Card>
        </div>
      </div>
    </ScrollArea>
  </div>
</template>
