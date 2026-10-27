export interface SyncableAccount {
  name: string
  business_id?: string
  is_default_outgoing?: boolean
}

/**
 * The accounts to sync templates for. Templates live on the WhatsApp Business
 * Account, so with "all" selected each business account is synced once,
 * through its default outgoing number when it has one.
 */
export function accountsToSync(accounts: SyncableAccount[], selected: string): string[] {
  if (selected && selected !== 'all') return [selected]
  const byBusiness = new Map<string, SyncableAccount>()
  for (const account of accounts) {
    const key = account.business_id || account.name
    const current = byBusiness.get(key)
    if (!current || (account.is_default_outgoing && !current.is_default_outgoing)) {
      byBusiness.set(key, account)
    }
  }
  return [...byBusiness.values()].map(a => a.name)
}
