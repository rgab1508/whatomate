import { describe, expect, it } from 'vitest'
import {
  addCard,
  addCardButton,
  carouselProblem,
  fromApi,
  newCarousel,
  removeCard,
  removeCardButton,
  setButtonType,
  setHeaderFormat,
  type CarouselCard,
} from './carousel'

function filled(): CarouselCard[] {
  return newCarousel().map((c, i) => ({
    ...c,
    header_handle: `4::h${i}`,
    media_id: `m${i}`,
    body: `Card ${i}`,
    buttons: [{ type: 'URL', text: 'Book', url: 'https://gharit.in/book' }],
  }))
}

describe('carousel editor helpers', () => {
  it('starts with two image cards sharing one URL button', () => {
    const cards = newCarousel()
    expect(cards).toHaveLength(2)
    expect(cards.every(c => c.header_format === 'IMAGE')).toBe(true)
    expect(cards.map(c => c.buttons.map(b => b.type))).toEqual([['URL'], ['URL']])
  })

  it('new cards copy the shared media type and button layout', () => {
    const cards = addCard(setButtonType(addCardButton(filled()), 1, 'PHONE_NUMBER'))
    expect(cards).toHaveLength(3)
    expect(cards[2].buttons.map(b => b.type)).toEqual(['URL', 'PHONE_NUMBER'])
    expect(cards[2].body).toBe('')
  })

  it('keeps 2 to 10 cards and 1 to 2 buttons', () => {
    expect(removeCard(filled(), 0)).toHaveLength(2)
    let cards = filled()
    for (let i = 0; i < 12; i++) cards = addCard(cards)
    expect(cards).toHaveLength(10)
    expect(removeCardButton(filled(), 0)[0].buttons).toHaveLength(1)
    expect(addCardButton(addCardButton(filled()))[0].buttons).toHaveLength(2)
  })

  it('changing the media type clears uploaded samples', () => {
    const cards = setHeaderFormat(filled(), 'VIDEO')
    expect(cards.every(c => c.header_format === 'VIDEO' && !c.header_handle && !c.media_id)).toBe(true)
    expect(setHeaderFormat(filled(), 'IMAGE')[0].media_id).toBe('m0')
  })

  it('changing a button type keeps each card label', () => {
    const cards = setButtonType(filled(), 0, 'QUICK_REPLY')
    expect(cards[1].buttons[0]).toEqual({ type: 'QUICK_REPLY', text: 'Book' })
  })

  it('names the first problem, like the server does', () => {
    expect(carouselProblem(filled())).toBeNull()
    expect(carouselProblem(filled().slice(0, 1))).toContain('2 to 10 cards')
    const noText = filled()
    noText[1].body = ' '
    expect(carouselProblem(noText)).toBe('Card 2: text is required')
    const noUrl = filled()
    noUrl[0].buttons[0].url = ''
    expect(carouselProblem(noUrl)).toBe('Card 1, button 1: URL is required')
  })

  it('reads cards from the API with blanks filled in', () => {
    const [card] = fromApi([{ header_format: 'image', body: 'Hi', buttons: [{ type: 'url', text: 'Go' }] }])
    expect(card.header_format).toBe('IMAGE')
    expect(card.media_id).toBe('')
    expect(card.buttons[0]).toEqual({ type: 'URL', text: 'Go', url: '' })
    expect(fromApi(undefined)).toEqual([])
  })
})
