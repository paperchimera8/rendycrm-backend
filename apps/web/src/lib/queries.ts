import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  assignConversation,
  blockDaySlot,
  cancelBooking,
  completeBooking,
  createDaySlot,
  createBooking,
  moveDaySlot,
  createSlotColor,
  createSlotTemplate,
  deleteDaySlot,
  deleteSlotColor,
  deleteSlotTemplate,
  getAvailableSlots,
  getBookings,
  getAnalytics,
  getAvailability,
  getSlotEditor,
  getBotConfig,
  getChannels,
  getConversation,
  getConversations,
  getDashboard,
  getMasterProfile,
  getMe,
  getOperatorBotSettings,
  getReviews,
  getWeekSlots,
  reopenConversation,
  reorderDaySlots,
  reorderSlotColors,
  reorderSlotTemplates,
  resolveConversation,
  updateCustomer,
  rescheduleBooking,
  replyToConversation,
  simulateWebhook,
  unblockDaySlot,
  unlinkOperatorBot,
  updateDaySlot,
  updateMasterProfile,
  updateBotConfig,
  updateChannel,
  updateChannelBot,
  updateOperatorBotSettings,
  createOperatorBotLinkCode,
  updateAvailabilityRules,
  updateSlotColor,
  updateSlotSettings,
  updateSlotTemplate,
  updateReviewStatus
} from './api'
import type { BotConfig, FAQItem } from './types'

export function useMe() {
  return useQuery({ queryKey: ['me'], queryFn: getMe, retry: false })
}

export function useDashboard() {
  return useQuery({ queryKey: ['dashboard'], queryFn: getDashboard })
}

export function useConversations() {
  return useQuery({ queryKey: ['conversations'], queryFn: getConversations })
}

export function useConversation(id: string | null) {
  return useQuery({ queryKey: ['conversation', id], queryFn: () => getConversation(id!), enabled: Boolean(id) })
}

export function useAvailability() {
  return useQuery({ queryKey: ['availability'], queryFn: getAvailability })
}

export function useSlotEditor(date: string) {
  return useQuery({ queryKey: ['slot-editor', date], queryFn: () => getSlotEditor(date) })
}

export function useWeekSlots(date: string) {
  return useQuery({ queryKey: ['week-slots', date], queryFn: () => getWeekSlots(date) })
}

export function useAvailableSlots(dateFrom: string, dateTo: string) {
  return useQuery({ queryKey: ['available-slots', dateFrom, dateTo], queryFn: () => getAvailableSlots(dateFrom, dateTo) })
}

export function useBookings(status = 'all') {
  return useQuery({ queryKey: ['bookings', status], queryFn: () => getBookings(status) })
}

export function useReviews() {
  return useQuery({ queryKey: ['reviews'], queryFn: getReviews })
}

export function useAnalytics() {
  return useQuery({ queryKey: ['analytics'], queryFn: getAnalytics })
}

export function useChannels() {
  return useQuery({ queryKey: ['channels'], queryFn: getChannels })
}

export function useMasterProfile() {
  return useQuery({ queryKey: ['master-profile'], queryFn: getMasterProfile })
}

export function useBotConfig() {
  return useQuery({ queryKey: ['bot-config'], queryFn: getBotConfig })
}

export function useOperatorBotSettings() {
  return useQuery({ queryKey: ['operator-bot'], queryFn: getOperatorBotSettings })
}

export function useReplyMutation() {
  return useMutation({
    mutationFn: ({ id, text }: { id: string; text: string }) => replyToConversation(id, text)
  })
}

export function useAssignMutation() {
  return useMutation({
    mutationFn: (id: string) => assignConversation(id)
  })
}

export function useResolveConversationMutation() {
  return useMutation({
    mutationFn: (id: string) => resolveConversation(id)
  })
}

export function useReopenConversationMutation() {
  return useMutation({
    mutationFn: (id: string) => reopenConversation(id)
  })
}

export function useUpdateCustomerMutation() {
  return useMutation({
    mutationFn: ({ id, name }: { id: string; name: string }) => updateCustomer(id, { name })
  })
}

export function useCreateBookingMutation() {
  return useMutation({
    mutationFn: ({
      customerId,
      dailySlotId,
      startsAt,
      endsAt,
      amount,
      status,
      notes,
      conversationId
    }: {
      customerId: string
      dailySlotId?: string
      startsAt?: string
      endsAt?: string
      amount?: number
      status?: 'pending' | 'confirmed'
      notes: string
      conversationId?: string
    }) => createBooking({ customerId, dailySlotId, startsAt, endsAt, amount, status, notes, conversationId })
  })
}

export function useCompleteBookingMutation() {
  return useMutation({
    mutationFn: ({ id, amount }: { id: string; amount: number }) => completeBooking(id, amount)
  })
}

export function useCancelBookingMutation() {
  return useMutation({
    mutationFn: ({ id, conversationId }: { id: string; conversationId?: string }) => cancelBooking(id, conversationId)
  })
}

export function useRescheduleBookingMutation() {
  return useMutation({
    mutationFn: ({
      id,
      dailySlotId,
      startsAt,
      endsAt,
      amount,
      status,
      notes,
      conversationId
    }: {
      id: string
      dailySlotId?: string
      startsAt?: string
      endsAt?: string
      amount?: number
      status?: 'pending' | 'confirmed'
      notes: string
      conversationId?: string
    }) => rescheduleBooking(id, { dailySlotId, startsAt, endsAt, amount, status, notes, conversationId })
  })
}

export function useUpdateSlotSettingsMutation() {
  return useMutation({
    mutationFn: updateSlotSettings
  })
}

export function useCreateSlotColorMutation() {
  return useMutation({
    mutationFn: ({ name, hex }: { name: string; hex: string }) => createSlotColor(name, hex)
  })
}

export function useUpdateSlotColorMutation() {
  return useMutation({
    mutationFn: ({ id, name, hex }: { id: string; name: string; hex: string }) => updateSlotColor(id, { name, hex })
  })
}

export function useReorderSlotColorsMutation() {
  return useMutation({
    mutationFn: reorderSlotColors
  })
}

export function useDeleteSlotColorMutation() {
  return useMutation({
    mutationFn: (id: string) => deleteSlotColor(id)
  })
}

export function useCreateSlotTemplateMutation() {
  return useMutation({
    mutationFn: createSlotTemplate
  })
}

export function useUpdateSlotTemplateMutation() {
  return useMutation({
    mutationFn: ({ id, ...payload }: { id: string; weekday: number; startMinute: number; durationMinutes: number; colorPresetId: string; enabled: boolean }) =>
      updateSlotTemplate(id, payload)
  })
}

export function useReorderSlotTemplatesMutation() {
  return useMutation({
    mutationFn: reorderSlotTemplates
  })
}

export function useDeleteSlotTemplateMutation() {
  return useMutation({
    mutationFn: (id: string) => deleteSlotTemplate(id)
  })
}

export function useCreateDaySlotMutation() {
  return useMutation({
    mutationFn: createDaySlot
  })
}

export function useUpdateDaySlotMutation() {
  return useMutation({
    mutationFn: ({ id, ...payload }: { id: string; slotDate: string; startsAt: string; durationMinutes: number; colorPresetId: string; note: string }) =>
      updateDaySlot(id, payload)
  })
}

export function useReorderDaySlotsMutation() {
  return useMutation({
    mutationFn: ({ slotDate, ids }: { slotDate: string; ids: string[] }) => reorderDaySlots(slotDate, ids)
  })
}

export function useDeleteDaySlotMutation() {
  return useMutation({
    mutationFn: (id: string) => deleteDaySlot(id)
  })
}

export function useMoveDaySlotMutation() {
  return useMutation({
    mutationFn: ({ id, targetSlotDate, targetIndex }: { id: string; targetSlotDate: string; targetIndex: number }) =>
      moveDaySlot({ id, targetSlotDate, targetIndex })
  })
}

export function useBlockDaySlotMutation() {
  return useMutation({
    mutationFn: (id: string) => blockDaySlot(id)
  })
}

export function useUnblockDaySlotMutation() {
  return useMutation({
    mutationFn: (id: string) => unblockDaySlot(id)
  })
}

export function useReviewStatusMutation() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: ({ id, status }: { id: string; status: 'open' | 'resolved' }) => updateReviewStatus(id, status),
    onSuccess: () => {
      client.invalidateQueries({ queryKey: ['reviews'] })
      client.invalidateQueries({ queryKey: ['dashboard'] })
    }
  })
}

export function useUpdateChannelMutation() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: ({ provider, connected, name }: { provider: string; connected: boolean; name: string }) =>
      updateChannel(provider, { connected, name }),
    onSuccess: () => client.invalidateQueries({ queryKey: ['channels'] })
  })
}

export function useUpdateChannelBotMutation() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: ({
      provider,
      connected,
      name,
      botUsername,
      botToken,
      webhookSecret
    }: {
      provider: string
      connected: boolean
      name: string
      botUsername: string
      botToken: string
      webhookSecret: string
    }) => updateChannelBot(provider, { connected, name, botUsername, botToken, webhookSecret }),
    onSuccess: () => client.invalidateQueries({ queryKey: ['channels'] })
  })
}

export function useUpdateMasterProfileMutation() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: ({ masterPhone }: { masterPhone: string }) => updateMasterProfile(masterPhone),
    onSuccess: () => client.invalidateQueries({ queryKey: ['master-profile'] })
  })
}

export function useUpdateBotMutation() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: ({ config, faqItems }: { config: BotConfig; faqItems: FAQItem[] }) =>
      updateBotConfig(config, faqItems),
    onSuccess: () => client.invalidateQueries({ queryKey: ['bot-config'] })
  })
}

export function useCreateOperatorBotLinkCodeMutation() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: createOperatorBotLinkCode,
    onSuccess: () => client.invalidateQueries({ queryKey: ['operator-bot'] })
  })
}

export function useUpdateOperatorBotMutation() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: ({
      enabled,
      botUsername,
      botToken,
      webhookSecret
    }: {
      enabled: boolean
      botUsername: string
      botToken: string
      webhookSecret: string
    }) => updateOperatorBotSettings({ enabled, botUsername, botToken, webhookSecret }),
    onSuccess: () => client.invalidateQueries({ queryKey: ['operator-bot'] })
  })
}

export function useUnlinkOperatorBotMutation() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: unlinkOperatorBot,
    onSuccess: () => client.invalidateQueries({ queryKey: ['operator-bot'] })
  })
}

export function useUpdateAvailabilityRulesMutation() {
  return useMutation({
    mutationFn: updateAvailabilityRules
  })
}

export function useSimulateWebhookMutation() {
  return useMutation({
    mutationFn: ({ provider, customerName, text }: { provider: 'telegram' | 'whatsapp'; customerName: string; text: string }) =>
      simulateWebhook(provider, customerName, text)
  })
}
