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
import { appUrl, defaultApiBaseUrl, stripAppBasePath } from './basePath'

const TOKEN_KEY = 'rendycrm.token'
const runtimeApiBase = window.RUNTIME_CONFIG?.API_BASE_URL?.trim()
const envApiBase = import.meta.env.VITE_API_BASE_URL?.trim()
const API_BASE_URL = (runtimeApiBase || envApiBase || defaultApiBaseUrl()).replace(/\/+$/, '')

export function apiUrl(path: string): string {
  if (!API_BASE_URL) return path
  if (path.startsWith('/')) return `${API_BASE_URL}${path}`
  return `${API_BASE_URL}/${path}`
}

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
    response = await fetch(apiUrl(path), { ...init, headers, credentials: 'include' })
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
      if (typeof window !== 'undefined' && stripAppBasePath(window.location.pathname) !== '/login') {
        window.location.assign(appUrl('/login'))
      }
      throw new Error(data.error ?? 'Сессия истекла. Войдите снова.')
    }
    throw new Error(data.error ?? 'Request failed')
  }
  return data as T
}

function asArray<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : []
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
  const data = await request<{
    user?: { id: string; name: string; email: string; role: string } | null
    workspace?: { id: string; name: string } | null
  }>('/auth/me')
  return {
    user: data.user ?? { id: '', name: '', email: '', role: '' },
    workspace: data.workspace ?? { id: '', name: '' }
  }
}

export async function getDashboard() {
  return request<Dashboard>('/dashboard')
}

export async function getConversations() {
  const data = await request<{ items?: Conversation[] | null }>('/conversations')
  return { items: asArray(data.items) }
}

export async function getConversation(id: string) {
  const data = await request<{ conversation: Conversation; messages?: Message[] | null; customer?: Customer | null }>(`/conversations/${id}`)
  return {
    ...data,
    messages: asArray(data.messages),
    customer:
      data.customer ??
      ({
        id: '',
        name: '',
        phone: '',
        email: '',
        notes: '',
        lastVisitAt: '',
        bookingCount: 0,
        preferredChannel: ''
      } as Customer)
  }
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
  const data = await request<{ rules?: AvailabilityRule[] | null; exceptions?: AvailabilityException[] | null; slots?: Slot[] | null }>('/availability')
  return {
    rules: asArray(data.rules),
    exceptions: asArray(data.exceptions),
    slots: asArray(data.slots)
  }
}

export async function getSlotEditor(date: string) {
  const data = await request<Partial<SlotEditorResponse>>(`/slots/editor?date=${encodeURIComponent(date)}`)
  return {
    settings: data.settings ?? {
      workspaceId: '',
      timezone: 'Europe/Moscow',
      defaultDurationMinutes: 60,
      generationHorizonDays: 30
    },
    colors: asArray(data.colors),
    weekTemplates: asArray(data.weekTemplates),
    daySlots: asArray(data.daySlots)
  }
}

export async function getWeekSlots(date: string) {
  const data = await request<{ days?: WeekSlotDay[] | null }>(`/slots/week?date=${encodeURIComponent(date)}`)
  const days = asArray(data.days).map((day) => ({ ...day, slots: asArray(day?.slots) }))
  return { days }
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
  const data = await request<{ items?: DailySlot[] | null }>(`/slots/available?${search.toString()}`)
  return { items: asArray(data.items) }
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
  const data = await request<{ items?: Booking[] | null }>(`/bookings${suffix}`)
  return { items: asArray(data.items) }
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
  const data = await request<{ items?: Review[] | null }>('/reviews')
  return { items: asArray(data.items) }
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
  const data = await request<{ items?: ChannelAccount[] | null }>('/settings/channels')
  return { items: asArray(data.items) }
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
  const data = await request<MasterProfile | null>('/settings/master-profile')
  return (
    data ?? {
      workspaceId: '',
      masterPhoneRaw: '',
      masterPhoneNormalized: '',
      telegramEnabled: false,
      clientBotUsername: '',
      clientBotDeepLink: ''
    }
  )
}

export async function updateMasterProfile(masterPhone: string) {
  return request<MasterProfile>('/settings/master-profile', {
    method: 'PUT',
    body: JSON.stringify({ masterPhone })
  })
}

export async function getBotConfig() {
  const data = await request<{ config?: BotConfig | null; faqItems?: FAQItem[] | null }>('/settings/bot')
  return {
    config: data.config ?? {
      workspaceId: '',
      autoReply: false,
      handoffEnabled: false,
      faqCount: 0,
      tone: 'helpful'
    },
    faqItems: asArray(data.faqItems)
  }
}

export async function updateBotConfig(config: BotConfig, faqItems: FAQItem[]) {
  return request<{ config: BotConfig; faqItems: FAQItem[] }>('/settings/bot', {
    method: 'PUT',
    body: JSON.stringify({ config, faqItems })
  })
}

export async function getOperatorBotSettings() {
  const data = await request<OperatorBotSettings | null>('/settings/operator-bot')
  return (
    data ?? {
      binding: null,
      pendingLink: null,
      botUsername: '',
      operatorWebhookUrl: '',
      tokenConfigured: false
    }
  )
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
