<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'
import { templatesService } from '@/services/api'
import { getErrorMessage } from '@/lib/api-utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Check, Loader2, Plus, Trash2, Upload, X } from 'lucide-vue-next'
import {
  CAROUSEL_MAX_BUTTONS,
  CAROUSEL_MAX_CARDS,
  CAROUSEL_MIN_CARDS,
  CARD_BODY_MAX,
  addCard,
  addCardButton,
  removeCard,
  removeCardButton,
  setButtonType,
  setHeaderFormat,
  type CarouselCard,
} from './carousel'

// Meta's media card carousel: every card shares the header format and the
// button layout (types, in order), so those are edited once and applied to
// all cards. Each card keeps its own media, text and button labels.

const props = defineProps<{
  modelValue: CarouselCard[]
  account: string
  disabled?: boolean
}>()

const emit = defineEmits<{ 'update:modelValue': [cards: CarouselCard[]] }>()

const { t } = useI18n()

const cards = computed(() => props.modelValue)
const headerFormat = computed(() => cards.value[0]?.header_format || 'IMAGE')
const buttonLayout = computed(() => cards.value[0]?.buttons || [])

const pendingFiles = ref<Record<number, File | null>>({})
const uploading = ref<Record<number, boolean>>({})

const buttonTypes = [
  { value: 'URL', label: 'URL' },
  { value: 'QUICK_REPLY', label: 'Quick Reply' },
  { value: 'PHONE_NUMBER', label: 'Phone Number' },
]

function update(next: CarouselCard[]) {
  emit('update:modelValue', next)
}

function onFile(index: number, event: Event) {
  const input = event.target as HTMLInputElement
  pendingFiles.value[index] = input.files?.[0] || null
}

async function upload(index: number) {
  const file = pendingFiles.value[index]
  if (!file) return
  if (!props.account) {
    toast.error(t('templates.selectAccountFirst', 'Select an account first'))
    return
  }
  uploading.value[index] = true
  try {
    const response = await templatesService.uploadMedia(props.account, file)
    const data = (response.data as any).data
    const next = cards.value.map((c, i) => i === index
      ? { ...c, header_handle: data.handle, media_id: data.media_id || '', filename: data.filename || file.name }
      : c)
    update(next)
    pendingFiles.value[index] = null
    if (!data.media_id) {
      toast.warning(t('templates.carouselNoMediaId', 'Uploaded for approval, but not for sending. Upload again before sending.'))
    }
  } catch (err) {
    toast.error(getErrorMessage(err, t('templates.uploadFailed', 'Upload failed')))
  } finally {
    uploading.value[index] = false
  }
}

function setCardField(index: number, field: 'body' | 'media_link', value: string | number) {
  update(cards.value.map((c, i) => i === index ? { ...c, [field]: String(value) } : c))
}

function setButtonField(cardIndex: number, buttonIndex: number, field: 'text' | 'url' | 'phone_number', value: string | number) {
  update(cards.value.map((c, i) => i !== cardIndex ? c : {
    ...c,
    buttons: c.buttons.map((b, j) => j === buttonIndex ? { ...b, [field]: String(value) } : b),
  }))
}

function acceptFor(format: string) {
  return format === 'VIDEO' ? 'video/mp4' : 'image/jpeg,image/png'
}
</script>

<template>
  <div class="space-y-4">
    <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
      <div class="space-y-1.5">
        <Label class="text-xs">{{ $t('templates.carouselMedia', 'Card media') }}</Label>
        <Select
          :model-value="headerFormat"
          :disabled="disabled"
          @update:model-value="(v) => update(setHeaderFormat(cards, String(v)))"
        >
          <SelectTrigger><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="IMAGE">{{ $t('templates.image', 'Image') }}</SelectItem>
            <SelectItem value="VIDEO">{{ $t('templates.video', 'Video') }}</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <div class="space-y-1.5">
        <Label class="text-xs">
          {{ $t('templates.carouselButtons', 'Buttons on every card') }}
          <span class="text-muted-foreground font-normal">(1–{{ CAROUSEL_MAX_BUTTONS }})</span>
        </Label>
        <div class="flex flex-wrap items-center gap-2">
          <div v-for="(btn, j) in buttonLayout" :key="j" class="flex items-center gap-1">
            <Select
              :model-value="btn.type"
              :disabled="disabled"
              @update:model-value="(v) => update(setButtonType(cards, j, String(v)))"
            >
              <SelectTrigger class="h-8 w-[140px] text-xs"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem v-for="bt in buttonTypes" :key="bt.value" :value="bt.value">{{ bt.label }}</SelectItem>
              </SelectContent>
            </Select>
            <Button
              v-if="!disabled && buttonLayout.length > 1"
              type="button"
              variant="ghost"
              size="icon"
              class="h-8 w-8"
              :aria-label="$t('templates.removeButton', 'Remove button')"
              @click="update(removeCardButton(cards, j))"
            >
              <X class="h-3.5 w-3.5" />
            </Button>
          </div>
          <Button
            v-if="!disabled && buttonLayout.length < CAROUSEL_MAX_BUTTONS"
            type="button"
            variant="outline"
            size="xs"
            class="h-8 text-xs"
            @click="update(addCardButton(cards))"
          >
            <Plus class="h-3 w-3 mr-1" /> {{ $t('templates.addButton', 'Add') }}
          </Button>
        </div>
      </div>
    </div>

    <div class="flex gap-3 overflow-x-auto pb-2 snap-x">
      <div
        v-for="(card, i) in cards"
        :key="i"
        class="snap-start shrink-0 w-[280px] max-w-[85vw] rounded-lg border p-3 space-y-3"
      >
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium">{{ $t('templates.card', 'Card') }} {{ i + 1 }}</span>
          <Button
            v-if="!disabled && cards.length > CAROUSEL_MIN_CARDS"
            type="button"
            variant="ghost"
            size="icon"
            class="h-7 w-7"
            :aria-label="$t('templates.removeCard', 'Remove card')"
            @click="update(removeCard(cards, i))"
          >
            <Trash2 class="h-3.5 w-3.5" />
          </Button>
        </div>

        <div class="space-y-1.5">
          <div class="flex items-center gap-2">
            <input
              type="file"
              :accept="acceptFor(headerFormat)"
              :disabled="disabled"
              class="min-w-0 flex-1 text-xs file:mr-2 file:py-1 file:px-2 file:rounded-md file:border-0 file:text-xs file:bg-primary file:text-primary-foreground cursor-pointer"
              @change="onFile(i, $event)"
            />
            <Button
              type="button"
              size="sm"
              class="h-7"
              :disabled="disabled || !pendingFiles[i] || uploading[i]"
              @click="upload(i)"
            >
              <Loader2 v-if="uploading[i]" class="h-3.5 w-3.5 animate-spin" />
              <Upload v-else class="h-3.5 w-3.5" />
            </Button>
          </div>
          <p v-if="card.header_handle" class="flex items-center gap-1 text-xs text-green-600">
            <Check class="h-3 w-3" />
            <span class="truncate">{{ card.filename || $t('templates.mediaUploadedSuccess', 'Media uploaded') }}</span>
          </p>
          <p v-else class="text-xs text-muted-foreground">
            {{ headerFormat === 'VIDEO' ? 'MP4, max 16MB' : 'JPEG or PNG, max 5MB' }}
          </p>
          <Input
            :model-value="card.media_link"
            :disabled="disabled"
            :placeholder="$t('templates.cardMediaLink', 'Public link for sending (optional)')"
            class="h-8 text-xs"
            @update:model-value="(v) => setCardField(i, 'media_link', v)"
          />
        </div>

        <div class="space-y-1">
          <Textarea
            :model-value="card.body"
            :rows="3"
            :maxlength="CARD_BODY_MAX"
            :disabled="disabled"
            :placeholder="$t('templates.cardText', 'Card text')"
            @update:model-value="(v) => setCardField(i, 'body', v)"
          />
          <p class="text-right text-[11px] text-muted-foreground">{{ card.body.length }}/{{ CARD_BODY_MAX }}</p>
        </div>

        <div v-for="(btn, j) in card.buttons" :key="j" class="space-y-1.5">
          <Input
            :model-value="btn.text"
            :maxlength="25"
            :disabled="disabled"
            :placeholder="`${$t('templates.buttonText', 'Button text')} (${btn.type === 'QUICK_REPLY' ? 'Quick Reply' : btn.type === 'URL' ? 'URL' : 'Phone'})`"
            class="h-8 text-xs"
            @update:model-value="(v) => setButtonField(i, j, 'text', v)"
          />
          <Input
            v-if="btn.type === 'URL'"
            :model-value="btn.url"
            :disabled="disabled"
            placeholder="https://"
            class="h-8 text-xs"
            @update:model-value="(v) => setButtonField(i, j, 'url', v)"
          />
          <Input
            v-else-if="btn.type === 'PHONE_NUMBER'"
            :model-value="btn.phone_number"
            :disabled="disabled"
            placeholder="+91…"
            class="h-8 text-xs"
            @update:model-value="(v) => setButtonField(i, j, 'phone_number', v)"
          />
        </div>
      </div>

      <button
        v-if="!disabled && cards.length < CAROUSEL_MAX_CARDS"
        type="button"
        class="snap-start shrink-0 w-[120px] rounded-lg border border-dashed text-xs text-muted-foreground hover:bg-muted flex flex-col items-center justify-center gap-1"
        @click="update(addCard(cards))"
      >
        <Plus class="h-4 w-4" />
        {{ $t('templates.addCard', 'Add card') }}
      </button>
    </div>
    <p class="text-xs text-muted-foreground">
      {{ $t('templates.carouselHint', '2–10 cards. Every card uses the same media type and buttons; text and links can differ. Card text and links can\'t use variables yet. Give each card a public media link so sends keep working after Meta drops uploads (30 days).') }}
    </p>
  </div>
</template>
