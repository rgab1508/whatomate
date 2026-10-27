import { describe, expect, it } from 'vitest'
import { accountsToSync } from './templateSync'

const gharit = [
  { name: 'GHARIT', business_id: 'waba-1', is_default_outgoing: true },
  { name: 'GHARIT 2', business_id: 'waba-1', is_default_outgoing: false },
]

describe('accountsToSync', () => {
  it('syncs the chosen account', () => {
    expect(accountsToSync(gharit, 'GHARIT 2')).toEqual(['GHARIT 2'])
  })

  it('with all accounts, syncs each business account once through its default number', () => {
    expect(accountsToSync([...gharit].reverse(), 'all')).toEqual(['GHARIT'])
    expect(accountsToSync([...gharit, { name: 'Other', business_id: 'waba-2' }], 'all')).toEqual(['GHARIT', 'Other'])
  })

  it('has nothing to sync without accounts', () => {
    expect(accountsToSync([], 'all')).toEqual([])
  })
})
