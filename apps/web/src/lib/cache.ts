import type { QueryKey } from '@tanstack/react-query'

const bookingScopeKeys: QueryKey[] = [
  ['available-slots'],
  ['slot-editor'],
  ['week-slots'],
  ['bookings', 'all'],
  ['bookings', 'pending'],
  ['dashboard'],
  ['analytics']
]

const slotScopeKeys: QueryKey[] = [
  ['week-slots'],
  ['slot-editor'],
  ['available-slots'],
  ['dashboard'],
  ['bookings']
]

function dedupeQueryKeys(keys: QueryKey[]) {
  const seen = new Set<string>()
  return keys.filter((queryKey) => {
    const signature = JSON.stringify(queryKey)
    if (seen.has(signature)) {
      return false
    }
    seen.add(signature)
    return true
  })
}

export function bookingMutationInvalidateKeys(conversationID?: string | null) {
  const keys = [...bookingScopeKeys]
  if (conversationID) {
    keys.push(['conversation', conversationID])
  }
  return dedupeQueryKeys(keys)
}

export function slotMutationInvalidateKeys() {
  return dedupeQueryKeys([...slotScopeKeys])
}
