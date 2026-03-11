import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { useUIStore } from '../stores/ui'
import { useActionRunner } from './actions'
import { bookingMutationInvalidateKeys } from './cache'

function createWrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>
  }
}

describe('useActionRunner', () => {
  it('invalidates every booking query after a successful mutation', async () => {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false }
      }
    })
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries').mockResolvedValue(undefined)
    const pushToast = vi.fn()
    useUIStore.setState({ pushToast })

    const { result } = renderHook(() => useActionRunner(), {
      wrapper: createWrapper(queryClient)
    })

    await act(async () => {
      const response = await result.current.runAction({
        key: 'confirm-booking',
        event: 'booking_confirmed_direct',
        execute: async () => 'ok',
        successMessage: 'Запись подтверждена.',
        invalidateKeys: bookingMutationInvalidateKeys('cnv_3')
      })
      expect(response).toBe('ok')
    })

    expect(invalidateSpy.mock.calls.map(([options]) => options?.queryKey)).toEqual(bookingMutationInvalidateKeys('cnv_3'))
    expect(pushToast).toHaveBeenCalledWith('success', 'Запись подтверждена.')
  })
})
