import { describe, expect, it } from 'vitest'
import { bookingMutationInvalidateKeys, slotMutationInvalidateKeys } from './cache'

describe('cache invalidation helpers', () => {
  it('returns the full booking mutation invalidation set', () => {
    expect(bookingMutationInvalidateKeys('cnv_1')).toEqual([
      ['available-slots'],
      ['slot-editor'],
      ['week-slots'],
      ['bookings', 'all'],
      ['bookings', 'pending'],
      ['dashboard'],
      ['analytics'],
      ['conversation', 'cnv_1']
    ])
  })

  it('omits conversation-specific refetch when there is no active conversation', () => {
    expect(bookingMutationInvalidateKeys()).toEqual([
      ['available-slots'],
      ['slot-editor'],
      ['week-slots'],
      ['bookings', 'all'],
      ['bookings', 'pending'],
      ['dashboard'],
      ['analytics']
    ])
  })

  it('returns the shared slot mutation invalidation set', () => {
    expect(slotMutationInvalidateKeys()).toEqual([
      ['week-slots'],
      ['slot-editor'],
      ['available-slots'],
      ['dashboard'],
      ['bookings']
    ])
  })
})
