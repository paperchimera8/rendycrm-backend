import type {
  Analytics,
  AvailabilityException,
  AvailabilityRule,
  BotConfig,
  Booking,
  ChannelAccount,
  Conversation,
  Customer,
  DailySlot,
  Dashboard,
  FAQItem,
  MasterProfile,
  Message,
  OperatorBotLinkCode,
  OperatorBotSettings,
  Review,
  SlotColorPreset,
  SlotEditorResponse,
  SlotSettings,
  SlotTemplate,
  Slot,
  WeekSlotDay
} from './types'

const TOKEN_KEY = 'rendycrm.token'

export function getToken(): string | null {
  try {
    return localStorage.getItem(TOKEN_KEY)
  } catch {
    return null
  }
}

export function setToken(token: string) {
  try {
    localStorage.setItem(TOKEN_KEY, token)
  } catch {
    // ignore storage errors in blocked/private contexts
  }
}

export function clearToken() {
  try {
    localStorage.removeItem(TOKEN_KEY)
  } catch {
    // ignore storage errors in blocked/private contexts
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  headers.set('Content-Type', 'application/json')
  const token = getToken()
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }
  let response: Response
  try {
    response = await fetch(path, { ...init, headers, credentials: 'same-origin' })
  } catch (error) {
    if (error instanceof Error && error.message.toLowerCase().includes('failed to fetch')) {
      throw new Error(`Не удалось выполнить запрос ${path}. Проверьте backend и dev proxy.`)
    }
    throw error
  }
  const data = (await response.json().catch(() => ({}))) as { error?: string }
  if (!response.ok) {
    if (response.status === 401) {
      clearToken()
      if (typeof window !== 'undefined' && !window.location.pathname.startsWith('/login')) {
        window.location.assign('/login')
      }
      throw new Error(data.error ?? 'Сессия истекла. Войдите снова.')
    }
    throw new Error(data.error ?? 'Request failed')
  }
  return data as T
}

export async function login(email: string, password: string) {
  return request<{ token: string; user: { id: string; name: string; role: string } }>('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email, password })
  })
}

export async function logout() {
  return request<{ ok: boolean }>('/auth/logout', { method: 'POST' })
}

export async function getMe() {
  return request<{ user: { id: string; name: string; email: string; role: string }; workspace: { id: string; name: string } }>('/auth/me')
}

export async function getDashboard() {
  return request<Dashboard>('/dashboard')
}

export async function getConversations() {
  return request<{ items: Conversation[] }>('/conversations')
}

export async function getConversation(id: string) {
  return request<{ conversation: Conversation; messages: Message[]; customer: Customer }>(`/conversations/${id}`)
}

export async function updateCustomer(id: string, payload: { name: string }) {
  return request<Customer>(`/customers/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(payload)
  })
}

export async function replyToConversation(id: string, text: string) {
  return request(`/conversations/${id}/reply`, { method: 'POST', body: JSON.stringify({ text }) })
}

export async function assignConversation(id: string) {
  return request(`/conversations/${id}/assign`, { method: 'POST' })
}

export async function resolveConversation(id: string) {
  return request(`/conversations/${id}/resolve`, { method: 'POST' })
}

export async function reopenConversation(id: string) {
  return request(`/conversations/${id}/reopen`, { method: 'POST' })
}

export async function getAvailability() {
  return request<{ rules: AvailabilityRule[]; exceptions: AvailabilityException[]; slots: Slot[] }>('/availability')
}

export async function getSlotEditor(date: string) {
  return request<SlotEditorResponse>(`/slots/editor?date=${encodeURIComponent(date)}`)
}

export async function getWeekSlots(date: string) {
  return request<{ days: WeekSlotDay[] }>(`/slots/week?date=${encodeURIComponent(date)}`)
}

export async function updateSlotSettings(payload: SlotSettings) {
  return request<SlotSettings>('/slots/settings', {
    method: 'PUT',
    body: JSON.stringify(payload)
  })
}

export async function createSlotColor(name: string, hex: string) {
  return request<SlotColorPreset>('/slots/colors', {
    method: 'POST',
    body: JSON.stringify({ name, hex })
  })
}

export async function updateSlotColor(id: string, payload: { name: string; hex: string }) {
  return request<SlotColorPreset>(`/slots/colors/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(payload)
  })
}

export async function reorderSlotColors(ids: string[]) {
  return request<{ ok: boolean }>('/slots/colors/reorder', {
    method: 'POST',
    body: JSON.stringify({ ids })
  })
}

export async function deleteSlotColor(id: string) {
  return request<{ ok: boolean }>(`/slots/colors/${id}`, {
    method: 'DELETE'
  })
}

export async function createSlotTemplate(payload: Pick<SlotTemplate, 'weekday' | 'startMinute' | 'durationMinutes' | 'colorPresetId' | 'enabled'>) {
  return request<SlotTemplate>('/slots/templates', {
    method: 'POST',
    body: JSON.stringify(payload)
  })
}

export async function updateSlotTemplate(id: string, payload: Pick<SlotTemplate, 'weekday' | 'startMinute' | 'durationMinutes' | 'colorPresetId' | 'enabled'>) {
  return request<SlotTemplate>(`/slots/templates/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(payload)
  })
}

export async function reorderSlotTemplates(ids: string[]) {
  return request<{ ok: boolean }>('/slots/templates/reorder', {
    method: 'POST',
    body: JSON.stringify({ ids })
  })
}

export async function deleteSlotTemplate(id: string) {
  return request<{ ok: boolean }>(`/slots/templates/${id}`, {
    method: 'DELETE'
  })
}

export async function createDaySlot(payload: { slotDate: string; startsAt: string; durationMinutes: number; colorPresetId: string; status: DailySlot['status']; note: string }) {
  return request<DailySlot>('/slots/day-slots', {
    method: 'POST',
    body: JSON.stringify(payload)
  })
}

export async function updateDaySlot(id: string, payload: { slotDate: string; startsAt: string; durationMinutes: number; colorPresetId: string; note: string }) {
  return request<DailySlot>(`/slots/day-slots/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(payload)
  })
}

export async function reorderDaySlots(slotDate: string, ids: string[]) {
  return request<{ ok: boolean }>('/slots/day-slots/reorder', {
    method: 'POST',
    body: JSON.stringify({ slotDate, ids })
  })
}

export async function moveDaySlot(payload: { id: string; targetSlotDate: string; targetIndex: number }) {
  return request<DailySlot>('/slots/day-slots/move', {
    method: 'POST',
    body: JSON.stringify(payload)
  })
}

export async function deleteDaySlot(id: string) {
  return request<{ ok: boolean }>(`/slots/day-slots/${id}`, { method: 'DELETE' })
}

export async function blockDaySlot(id: string) {
  return request<DailySlot>(`/slots/day-slots/${id}/block`, { method: 'POST', body: JSON.stringify({}) })
}

export async function unblockDaySlot(id: string) {
  return request<DailySlot>(`/slots/day-slots/${id}/unblock`, { method: 'POST', body: JSON.stringify({}) })
}

export async function getAvailableSlots(dateFrom: string, dateTo: string) {
  const search = new URLSearchParams({ date_from: dateFrom, date_to: dateTo })
  return request<{ items: DailySlot[] }>(`/slots/available?${search.toString()}`)
}

export async function updateAvailabilityRules(rules: AvailabilityRule[]) {
  return request<{ rules: AvailabilityRule[] }>('/availability/rules', {
    method: 'PUT',
    body: JSON.stringify({ rules })
  })
}

export async function updateAvailabilityExceptions(exceptions: AvailabilityException[]) {
  return request<{ exceptions: AvailabilityException[] }>('/availability/exceptions', {
    method: 'PUT',
    body: JSON.stringify({ exceptions })
  })
}

export async function createBooking(payload: { customerId: string; dailySlotId?: string; startsAt?: string; endsAt?: string; amount?: number; status?: 'pending' | 'confirmed'; notes: string; conversationId?: string }) {
  return request('/bookings', {
    method: 'POST',
    body: JSON.stringify(payload)
  })
}

export async function getBookings(status = 'all') {
  const search = new URLSearchParams()
  if (status && status !== 'all') {
    search.set('status', status)
  }
  const suffix = search.toString() ? `?${search.toString()}` : ''
  return request<{ items: Booking[] }>(`/bookings${suffix}`)
}

export async function confirmBooking(id: string, amount: number, conversationId?: string) {
  return request(`/bookings/${id}/confirm`, { method: 'POST', body: JSON.stringify({ amount, conversationId }) })
}

export async function completeBooking(id: string, amount: number) {
  return request(`/bookings/${id}/complete`, { method: 'POST', body: JSON.stringify({ amount }) })
}

export async function cancelBooking(id: string, conversationId?: string) {
  return request(`/bookings/${id}/cancel`, { method: 'POST', body: JSON.stringify({ conversationId }) })
}

export async function rescheduleBooking(id: string, payload: { dailySlotId?: string; startsAt?: string; endsAt?: string; amount?: number; status?: 'pending' | 'confirmed'; notes: string; conversationId?: string }) {
  return request(`/bookings/${id}/reschedule`, {
    method: 'POST',
    body: JSON.stringify(payload)
  })
}

export async function getReviews() {
  return request<{ items: Review[] }>('/reviews')
}

export async function updateReviewStatus(id: string, status: 'open' | 'resolved') {
  return request(`/reviews/${id}/status`, {
    method: 'POST',
    body: JSON.stringify({ status })
  })
}

export async function getAnalytics() {
  return request<Analytics>('/analytics/overview')
}

export async function getChannels() {
  return request<{ items: ChannelAccount[] }>('/settings/channels')
}

export async function updateChannel(provider: string, payload: { connected: boolean; name: string }) {
  return request<ChannelAccount>(`/settings/channels/${provider}`, {
    method: 'PUT',
    body: JSON.stringify(payload)
  })
}

export async function updateChannelBot(
  provider: string,
  payload: { connected: boolean; name: string; botUsername: string; botToken: string; webhookSecret: string }
) {
  return request<ChannelAccount>(`/settings/channels/${provider}`, {
    method: 'PUT',
    body: JSON.stringify(payload)
  })
}

export async function getMasterProfile() {
  return request<MasterProfile>('/settings/master-profile')
}

export async function updateMasterProfile(masterPhone: string) {
  return request<MasterProfile>('/settings/master-profile', {
    method: 'PUT',
    body: JSON.stringify({ masterPhone })
  })
}

export async function getBotConfig() {
  return request<{ config: BotConfig; faqItems: FAQItem[] }>('/settings/bot')
}

export async function updateBotConfig(config: BotConfig, faqItems: FAQItem[]) {
  return request<{ config: BotConfig; faqItems: FAQItem[] }>('/settings/bot', {
    method: 'PUT',
    body: JSON.stringify({ config, faqItems })
  })
}

export async function getOperatorBotSettings() {
  return request<OperatorBotSettings>('/settings/operator-bot')
}

export async function updateOperatorBotSettings(payload: {
  enabled: boolean
  botUsername: string
  botToken: string
  webhookSecret: string
}) {
  return request<OperatorBotSettings>('/settings/operator-bot', {
    method: 'PUT',
    body: JSON.stringify(payload)
  })
}

export async function createOperatorBotLinkCode() {
  return request<OperatorBotLinkCode>('/settings/operator-bot/link-code', {
    method: 'POST',
    body: JSON.stringify({})
  })
}

export async function unlinkOperatorBot() {
  return request<{ ok: boolean }>('/settings/operator-bot/unlink', {
    method: 'POST',
    body: JSON.stringify({})
  })
}

export async function simulateWebhook(provider: 'telegram' | 'whatsapp', customerName: string, text: string) {
  return request(`/webhooks/${provider}`, {
    method: 'POST',
    body: JSON.stringify({ customerName, text })
  })
}
