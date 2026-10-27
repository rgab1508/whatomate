// Media card carousel templates: pure helpers for the editor. The rules mirror
// pkg/whatsapp/carousel.go — every card shares the header format and the
// button layout (types, in order).

export interface CarouselButton {
  type: string
  text: string
  url?: string
  phone_number?: string
}

export interface CarouselCard {
  header_format: string
  header_handle: string
  media_id: string
  /** Public URL used when sending; preferred over media_id, which Meta drops after 30 days. */
  media_link: string
  filename: string
  body: string
  buttons: CarouselButton[]
}

export const CAROUSEL_MIN_CARDS = 2
export const CAROUSEL_MAX_CARDS = 10
export const CAROUSEL_MAX_BUTTONS = 2
export const CARD_BODY_MAX = 160

function blankButton(type: string): CarouselButton {
  if (type === 'URL') return { type, text: '', url: '' }
  if (type === 'PHONE_NUMBER') return { type, text: '', phone_number: '' }
  return { type, text: '' }
}

function blankCard(headerFormat: string, layout: string[]): CarouselCard {
  return {
    header_format: headerFormat,
    header_handle: '',
    media_id: '',
    media_link: '',
    filename: '',
    body: '',
    buttons: layout.map(blankButton),
  }
}

function layoutOf(cards: CarouselCard[]): string[] {
  return (cards[0]?.buttons || []).map(b => b.type)
}

/** Two empty image cards with one URL button: the starting point. */
export function newCarousel(): CarouselCard[] {
  return [blankCard('IMAGE', ['URL']), blankCard('IMAGE', ['URL'])]
}

/** Normalises cards loaded from the API (missing fields become blanks). */
export function fromApi(raw: any[] | undefined | null): CarouselCard[] {
  return (raw || []).map((c: any) => ({
    header_format: (c?.header_format || 'IMAGE').toUpperCase(),
    header_handle: c?.header_handle || '',
    media_id: c?.media_id || '',
    media_link: c?.media_link || '',
    filename: c?.filename || '',
    body: c?.body || '',
    buttons: (c?.buttons || []).map((b: any) => ({ ...blankButton((b?.type || 'QUICK_REPLY').toUpperCase()), ...b, type: (b?.type || 'QUICK_REPLY').toUpperCase() })),
  }))
}

export function addCard(cards: CarouselCard[]): CarouselCard[] {
  if (cards.length >= CAROUSEL_MAX_CARDS) return cards
  return [...cards, blankCard(cards[0]?.header_format || 'IMAGE', layoutOf(cards))]
}

export function removeCard(cards: CarouselCard[], index: number): CarouselCard[] {
  if (cards.length <= CAROUSEL_MIN_CARDS) return cards
  return cards.filter((_, i) => i !== index)
}

/** Changing the media type drops uploaded samples: an image can't approve a video card. */
export function setHeaderFormat(cards: CarouselCard[], format: string): CarouselCard[] {
  if (cards[0]?.header_format === format) return cards
  return cards.map(c => ({ ...c, header_format: format, header_handle: '', media_id: '', media_link: '', filename: '' }))
}

export function addCardButton(cards: CarouselCard[]): CarouselCard[] {
  if (layoutOf(cards).length >= CAROUSEL_MAX_BUTTONS) return cards
  return cards.map(c => ({ ...c, buttons: [...c.buttons, blankButton('QUICK_REPLY')] }))
}

export function removeCardButton(cards: CarouselCard[], index: number): CarouselCard[] {
  if (layoutOf(cards).length <= 1) return cards
  return cards.map(c => ({ ...c, buttons: c.buttons.filter((_, j) => j !== index) }))
}

/** Sets one button's type on every card, keeping each card's label. */
export function setButtonType(cards: CarouselCard[], index: number, type: string): CarouselCard[] {
  return cards.map(c => ({
    ...c,
    buttons: c.buttons.map((b, j) => j === index ? { ...blankButton(type), text: b.text } : b),
  }))
}

/** First problem to fix before saving, or null. Mirrors the server's checks. */
export function carouselProblem(cards: CarouselCard[]): string | null {
  if (cards.length < CAROUSEL_MIN_CARDS || cards.length > CAROUSEL_MAX_CARDS) {
    return `A carousel needs ${CAROUSEL_MIN_CARDS} to ${CAROUSEL_MAX_CARDS} cards`
  }
  for (const [i, c] of cards.entries()) {
    const n = i + 1
    if (!c.body.trim()) return `Card ${n}: text is required`
    if (c.body.includes('{{')) return `Card ${n}: variables aren't supported in card text yet`
    for (const [j, b] of c.buttons.entries()) {
      if (!b.text.trim()) return `Card ${n}, button ${j + 1}: text is required`
      if (b.type === 'URL' && !b.url?.trim()) return `Card ${n}, button ${j + 1}: URL is required`
      if (b.type === 'PHONE_NUMBER' && !b.phone_number?.trim()) return `Card ${n}, button ${j + 1}: phone number is required`
    }
  }
  return null
}
