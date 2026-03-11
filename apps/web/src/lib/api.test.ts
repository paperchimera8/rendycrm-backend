import { describe, expect, it, vi } from 'vitest'
import { getDashboard, getToken, setToken } from './api'

describe('api request handling', () => {
  it('clears the token and returns an auth error after 401', async () => {
    setToken('session-token')
    window.history.pushState({}, '', '/login')
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: 'Сессия истекла. Войдите снова.' }), {
          status: 401,
          headers: { 'Content-Type': 'application/json' }
        })
      )
    )

    await expect(getDashboard()).rejects.toThrow('Сессия истекла. Войдите снова.')

    expect(getToken()).toBeNull()
  })

  it('returns a helpful message for network fetch failures', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('Failed to fetch')))

    await expect(getDashboard()).rejects.toThrow('Не удалось выполнить запрос /dashboard. Проверьте backend и dev proxy.')
  })
})
