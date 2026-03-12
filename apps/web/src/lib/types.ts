export type Conversation = {
  id: string
  customerId: string
  provider: 'telegram' | 'whatsapp'
  channel?: 'telegram' | 'whatsapp'
  externalChatId?: string
  title: string
  status: 'new' | 'auto' | 'human' | 'booked' | 'closed'
  assignedUserId: string
  assignedOperatorId?: string
  unreadCount: number
  aiSummary: string
  intent?: 'booking_request' | 'availability_question' | 'price_question' | 'reschedule' | 'cancel' | 'faq' | 'human_request' | 'other'
  lastMessageText: string
  lastInboundAt?: string
  lastOutboundAt?: string
  updatedAt: string
}

export type Message = {
  id: string
  conversationId: string
  direction: 'inbound' | 'outbound'
  senderType: 'customer' | 'operator' | 'bot'
  text: string
  status: string
  deliveryStatus?: string
  externalId?: string
  createdAt: string
}

export type CustomerChannelIdentity = {
  id: string
  provider: 'telegram' | 'whatsapp'
  externalId: string
  username: string
  displayName: string
}

export type Customer = {
  id: string
  name: string
  phone: string
  email: string
  notes: string
  lastVisitAt: string
  bookingCount: number
  preferredChannel: string
  channels?: CustomerChannelIdentity[]
}

export type Booking = {
  id: string
  customerId: string
  customerName: string
  dailySlotId: string
  startsAt: string
  endsAt: string
  amount: number
  status: 'pending' | 'confirmed' | 'completed' | 'cancelled'
  source: string
  notes: string
}

export type Dashboard = {
  todayBookings: number
  awaitingConfirmation: number
  cancelledBookings: number
  newMessages: number
  newReviews: number
  activeConversations: number
}

export type AvailabilityRule = {
  id: string
  dayOfWeek: number
  startMinute: number
  endMinute: number
  enabled: boolean
}

export type AvailabilityException = {
  id: string
  startsAt: string
  endsAt: string
  reason: string
}

export type Slot = {
  start: string
  end: string
}

export type SlotSettings = {
  workspaceId: string
  timezone: string
  defaultDurationMinutes: number
  generationHorizonDays: number
}

export type SlotColorPreset = {
  id: string
  workspaceId: string
  name: string
  hex: string
  position: number
}

export type SlotTemplate = {
  id: string
  workspaceId: string
  weekday: number
  startMinute: number
  durationMinutes: number
  colorPresetId: string
  colorName: string
  colorHex: string
  position: number
  enabled: boolean
}

export type DailySlotStatus = 'free' | 'held' | 'booked' | 'blocked'

export type DailySlot = {
  id: string
  workspaceId: string
  slotDate: string
  startsAt: string
  endsAt: string
  durationMinutes: number
  colorPresetId: string
  colorName: string
  colorHex: string
  position: number
  status: DailySlotStatus
  sourceTemplateId: string
  isManual: boolean
  note: string
  bookingId: string
  customerName: string
}

export type WeekSlotDay = {
  date: string
  weekday: number
  label: string
  slots: DailySlot[]
}

export type SlotEditorResponse = {
  settings: SlotSettings
  colors: SlotColorPreset[]
  weekTemplates: SlotTemplate[]
  daySlots: DailySlot[]
}

export type Review = {
  id: string
  customerId: string
  bookingId: string
  rating: number
  text: string
  status: 'open' | 'resolved'
  createdAt: string
}

export type Analytics = {
  revenue: number
  confirmationRate: number
  noShowRate: number
  repeatBookings: number
  conversationToBooking: number
}

export type ChannelAccount = {
  id: string
  provider: 'telegram' | 'whatsapp'
  channel?: 'telegram' | 'whatsapp'
  channelKind?: 'telegram_client' | 'telegram_operator' | 'whatsapp_twilio'
  name: string
  connected: boolean
  isEnabled?: boolean
  webhookUrl: string
  accountId?: string
  botUsername?: string
  tokenConfigured?: boolean
}

export type MasterProfile = {
  workspaceId: string
  masterPhoneRaw: string
  masterPhoneNormalized: string
  telegramEnabled: boolean
}

export type BotConfig = {
  workspaceId: string
  autoReply: boolean
  handoffEnabled: boolean
  faqCount: number
  tone: string
  welcomeMessage?: string
  handoffMessage?: string
}

export type FAQItem = {
  id: string
  question: string
  answer: string
}

export type OperatorBotBinding = {
  userId: string
  workspaceId: string
  telegramUserId: string
  telegramChatId: string
  linkedAt: string
  isActive: boolean
}

export type OperatorBotLinkCode = {
  id: string
  userId: string
  workspaceId: string
  code: string
  expiresAt: string
  deepLink: string
}

export type OperatorBotSettings = {
  binding: OperatorBotBinding | null
  pendingLink: OperatorBotLinkCode | null
  botUsername: string
  operatorWebhookUrl: string
  tokenConfigured: boolean
}
