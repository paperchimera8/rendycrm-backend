import { useEffect, useMemo, useState } from 'react'
import { useRouter } from '@tanstack/react-router'
import { ActionButton } from '../components/ActionButton'
import { EmptyState } from '../components/EmptyState'
import { PageHeader } from '../components/PageHeader'
import { normalizeActionError, useActionRunner } from '../lib/actions'
import { bookingMutationInvalidateKeys } from '../lib/cache'
import {
  useAvailableSlots,
  useBookings,
  useCancelBookingMutation,
  useCompleteBookingMutation,
  useConversation,
  useConversations,
  useCreateBookingMutation,
  useReplyMutation,
  useRescheduleBookingMutation,
  useUpdateCustomerMutation
} from '../lib/queries'
import type { DailySlot } from '../lib/types'
import { useUIStore } from '../stores/ui'
import { cn } from '@/lib/utils'

const quickReplies = [
  'Есть подходящие слоты. Могу подтвердить ближайшее время или сразу перенести запись.',
  'Подтверждаю запись. Отправлю детали и напоминание перед визитом.',
  'Могу предложить другое время. Смотрите ближайшие свободные окна ниже.',
  'Если это окно не подходит, перенесу на удобное для клиента время без потери записи.'
]

function bookingStatusLabel(status: string) {
  if (status === 'pending') return 'ожидает'
  if (status === 'confirmed') return 'подтверждена'
  if (status === 'completed') return 'завершена'
  if (status === 'cancelled') return 'отменена'
  return status
}

function providerLabel(provider: string) {
  if (provider === 'telegram') return 'Telegram'
  if (provider === 'whatsapp') return 'WhatsApp'
  return provider
}

function formatTime(date: Date) {
  return date.toLocaleTimeString('ru', { hour: '2-digit', minute: '2-digit' })
}

function toDateTimeLocal(value: string) {
  const date = new Date(value)
  const year = date.getFullYear()
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  const hours = `${date.getHours()}`.padStart(2, '0')
  const minutes = `${date.getMinutes()}`.padStart(2, '0')
  return `${year}-${month}-${day}T${hours}:${minutes}`
}

function toISOStringFromLocal(value: string) {
  return new Date(value).toISOString()
}

function todayLocal() {
  const now = new Date()
  const year = now.getFullYear()
  const month = `${now.getMonth() + 1}`.padStart(2, '0')
  const day = `${now.getDate()}`.padStart(2, '0')
  return `${year}-${month}-${day}`
}

function addDaysLocal(days: number) {
  const date = new Date()
  date.setDate(date.getDate() + days)
  const year = date.getFullYear()
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  return `${year}-${month}-${day}`
}

function groupSlots(items: DailySlot[]) {
  return items.reduce<Record<string, DailySlot[]>>((acc, slot) => {
    acc[slot.slotDate] = [...(acc[slot.slotDate] ?? []), slot]
    return acc
  }, {})
}

function slotLabel(slot: DailySlot) {
  return `${new Date(slot.startsAt).toLocaleDateString('ru', { day: 'numeric', month: 'short' })} · ${new Date(slot.startsAt).toLocaleTimeString('ru', { hour: '2-digit', minute: '2-digit' })}`
}

export function DialogsPage() {
  const router = useRouter()
  const conversations = useConversations()
  const allBookings = useBookings('all')
  const availableSlots = useAvailableSlots(addDaysLocal(-1), addDaysLocal(30))
  const selectedConversationId = useUIStore((s) => s.selectedConversationId)
  const setSelectedConversationId = useUIStore((s) => s.setSelectedConversationId)
  const composeDraft = useUIStore((s) => s.composeDraft)
  const setComposeDraft = useUIStore((s) => s.setComposeDraft)
  const detail = useConversation(selectedConversationId)
  const replyMutation = useReplyMutation()
  const bookingMutation = useCreateBookingMutation()
  const completeBookingMutation = useCompleteBookingMutation()
  const cancelBookingMutation = useCancelBookingMutation()
  const rescheduleBookingMutation = useRescheduleBookingMutation()
  const updateCustomerMutation = useUpdateCustomerMutation()
  const { runAction, isPending } = useActionRunner()
  const pushToast = useUIStore((s) => s.pushToast)
  const [showHistory, setShowHistory] = useState(false)
  const [showReplyStack, setShowReplyStack] = useState(false)
  const [selectedSlotId, setSelectedSlotId] = useState('')
  const [bookingStartDraft, setBookingStartDraft] = useState('')
  const [bookingEndDraft, setBookingEndDraft] = useState('')
  const [bookingAmountDraft, setBookingAmountDraft] = useState('')
  const [customerNameDraft, setCustomerNameDraft] = useState('')

  const filteredConversations = useMemo(() => conversations.data?.items ?? [], [conversations.data?.items])

  useEffect(() => {
    if (!filteredConversations.length) {
      if (selectedConversationId !== null) setSelectedConversationId(null)
      return
    }
    if (!selectedConversationId || !filteredConversations.some((i) => i.id === selectedConversationId)) {
      setSelectedConversationId(filteredConversations[0].id)
    }
  }, [filteredConversations, selectedConversationId, setSelectedConversationId])

  const customerBookings = useMemo(() => {
    const customerId = detail.data?.customer.id
    if (!customerId) return []
    return [...(allBookings.data?.items ?? [])]
      .filter((booking) => booking.customerId === customerId)
      .sort((a, b) => new Date(b.startsAt).getTime() - new Date(a.startsAt).getTime())
  }, [allBookings.data?.items, detail.data?.customer.id])

  const latestPendingBooking = useMemo(() => customerBookings.find((booking) => booking.status === 'pending'), [customerBookings])
  const latestConfirmedBooking = useMemo(() => customerBookings.find((booking) => booking.status === 'confirmed'), [customerBookings])
  const latestActionableBooking = latestPendingBooking ?? latestConfirmedBooking ?? null
  const visibleAvailableSlots = useMemo(() => {
    const direct = availableSlots.data?.items ?? []
    const excludeDemo = /новый лид|demo flow/i
    const merged = new Map<string, DailySlot>()
    for (const slot of direct) {
      const text = `${slot.note || ''} ${slot.customerName || ''}`.toLowerCase()
      if (excludeDemo.test(text)) continue
      merged.set(slot.id, slot)
    }
    return [...merged.values()].sort((a, b) => new Date(a.startsAt).getTime() - new Date(b.startsAt).getTime())
  }, [availableSlots.data?.items])

  const groupedVisibleSlots = useMemo(() => groupSlots(visibleAvailableSlots), [visibleAvailableSlots])

  useEffect(() => {
    if (!latestActionableBooking) {
      setSelectedSlotId('')
      setBookingStartDraft('')
      setBookingEndDraft('')
      setBookingAmountDraft('')
      return
    }
    setSelectedSlotId(latestActionableBooking.dailySlotId || '')
    setBookingStartDraft(toDateTimeLocal(latestActionableBooking.startsAt))
    setBookingEndDraft(toDateTimeLocal(latestActionableBooking.endsAt))
    setBookingAmountDraft(latestActionableBooking.amount > 0 ? String(latestActionableBooking.amount) : '')
  }, [latestActionableBooking?.id, latestActionableBooking?.amount, latestActionableBooking?.startsAt, latestActionableBooking?.endsAt, latestActionableBooking?.dailySlotId])

  useEffect(() => {
    if (!detail.data?.customer.id) {
      setCustomerNameDraft('')
      return
    }
    setCustomerNameDraft(detail.data.customer.name)
  }, [detail.data?.customer.id])

  const parseBookingAmount = () => {
    const amount = Number(bookingAmountDraft.replace(',', '.'))
    if (!Number.isFinite(amount) || amount <= 0) {
      throw new Error('Укажите сумму записи в рублях.')
    }
    return Math.round(amount)
  }

  const parseBookingWindow = () => {
    if (!bookingStartDraft || !bookingEndDraft) {
      throw new Error('Укажите интервал записи.')
    }
    const startsAt = toISOStringFromLocal(bookingStartDraft)
    const endsAt = toISOStringFromLocal(bookingEndDraft)
    if (new Date(endsAt).getTime() <= new Date(startsAt).getTime()) {
      throw new Error('Время окончания должно быть позже времени начала.')
    }
    return { startsAt, endsAt }
  }

  const onReply = async () => {
    if (!selectedConversationId || !composeDraft.trim()) return
    await runAction({
      key: 'reply',
      event: 'reply_sent',
      execute: () => replyMutation.mutateAsync({ id: selectedConversationId, text: composeDraft }),
      successMessage: 'Ответ отправлен.',
      invalidateKeys: [['conversation', selectedConversationId], ['conversations'], ['dashboard']],
      telemetry: { screen: 'dialogs', entityId: selectedConversationId },
      onSuccess: () => setComposeDraft('')
    })
  }

  const onApplyAvailableSlot = (slot: DailySlot) => {
    setSelectedSlotId(slot.id)
    setBookingStartDraft(toDateTimeLocal(slot.startsAt))
    setBookingEndDraft(toDateTimeLocal(slot.endsAt))
  }

  const onSubmitBooking = async () => {
    if (!detail.data?.customer.id) return
    let amount = 0
    let startsAt = ''
    let endsAt = ''
    try {
      amount = parseBookingAmount()
      const window = parseBookingWindow()
      startsAt = window.startsAt
      endsAt = window.endsAt
    } catch (error) {
      pushToast('error', normalizeActionError(error))
      return
    }
    const selectedSlot = visibleAvailableSlots.find((slot) => slot.id === selectedSlotId)
    const selectedDailySlotId = selectedSlotId && !selectedSlotId.startsWith('avl_') ? selectedSlotId : undefined
    const notes = selectedSlot ? `Запись из диалога на ${slotLabel(selectedSlot)}` : 'Запись из диалога'

    if (!latestActionableBooking) {
      await runAction({
        key: 'create-confirmed-booking',
        event: 'booking_confirmed_direct',
        execute: () =>
          bookingMutation.mutateAsync({
            customerId: detail.data.customer.id,
            dailySlotId: selectedDailySlotId,
            startsAt,
            endsAt,
            amount,
            status: 'confirmed',
            notes,
            conversationId: selectedConversationId ?? undefined
          }),
        successMessage: 'Запись подтверждена.',
        invalidateKeys: bookingMutationInvalidateKeys(selectedConversationId),
        telemetry: { screen: 'dialogs', entityId: detail.data.customer.id }
      })
      return
    }

    await runAction({
      key: `reschedule-confirmed-booking-${latestActionableBooking.id}`,
      event: latestActionableBooking.status === 'pending' ? 'booking_confirmed_from_pending' : 'booking_rescheduled_confirmed',
      execute: () =>
        rescheduleBookingMutation.mutateAsync({
          id: latestActionableBooking.id,
          dailySlotId: selectedDailySlotId,
          startsAt,
          endsAt,
          amount,
          status: 'confirmed',
          notes,
          conversationId: selectedConversationId ?? undefined
        }),
      successMessage: latestActionableBooking.status === 'pending' ? 'Запись подтверждена.' : 'Запись перенесена.',
      invalidateKeys: bookingMutationInvalidateKeys(selectedConversationId),
      telemetry: { screen: 'dialogs', entityId: latestActionableBooking.id }
    })
  }

  const onCompleteLatestBooking = async () => {
    if (!latestConfirmedBooking) return
    let amount = 0
    try {
      amount = parseBookingAmount()
    } catch (error) {
      pushToast('error', normalizeActionError(error))
      return
    }
    await runAction({
      key: `complete-booking-${latestConfirmedBooking.id}`,
      event: 'booking_completed',
      execute: () => completeBookingMutation.mutateAsync({ id: latestConfirmedBooking.id, amount }),
      successMessage: 'Запись завершена.',
      invalidateKeys: bookingMutationInvalidateKeys(selectedConversationId),
      telemetry: { screen: 'dialogs', entityId: latestConfirmedBooking.id }
    })
  }

  const onCancelLatestBooking = async () => {
    if (!latestActionableBooking) return
    await runAction({
      key: `cancel-booking-${latestActionableBooking.id}`,
      event: 'booking_cancelled',
      execute: () => cancelBookingMutation.mutateAsync({ id: latestActionableBooking.id, conversationId: selectedConversationId ?? undefined }),
      successMessage: 'Запись отменена.',
      invalidateKeys: bookingMutationInvalidateKeys(),
      telemetry: { screen: 'dialogs', entityId: latestActionableBooking.id }
    })
  }

  const onUseTemplate = (template: string) => {
    setComposeDraft(composeDraft.trim() ? `${composeDraft.trim()}\n\n${template}` : template)
    setShowReplyStack(false)
  }

  const onSaveCustomerName = async () => {
    const customerId = detail.data?.customer.id
    const name = customerNameDraft.trim()
    if (!customerId) return
    if (!name) {
      return
    }
    const saved = await runAction({
      key: `customer-name-${customerId}`,
      event: 'customer_name_saved',
      execute: () => updateCustomerMutation.mutateAsync({ id: customerId, name }),
      successMessage: 'Имя клиента сохранено.',
      invalidateKeys: [['conversation', selectedConversationId], ['conversations'], ['bookings', 'all'], ['week-slots'], ['dashboard']],
      telemetry: { screen: 'dialogs', entityId: customerId }
    })
    if (saved) {
      setCustomerNameDraft(saved.name)
    }
  }

  return (
    <section className="flex h-[calc(100vh-8rem)] min-h-0 flex-col">
      <PageHeader title="Диалоги" />

      <div className="mt-2 flex min-h-0 flex-1 gap-0 overflow-hidden rounded-lg border border-[#ebebeb] bg-white">
        <aside className="flex w-[340px] shrink-0 flex-col border-r border-[#ebebeb]">
          {filteredConversations.length ? (
            <div className="flex-1 overflow-y-auto">
              {filteredConversations.map((conversation) => (
                <button
                  key={conversation.id}
                  type="button"
                  data-testid={`conversation-${conversation.id}`}
                  onClick={() => setSelectedConversationId(conversation.id)}
                  className={cn(
                    'w-full border-b border-[#f5f5f5] border-l-2 px-3 py-2.5 text-left transition-colors last:border-b-0',
                    selectedConversationId === conversation.id ? 'border-l-[#6c00a2] bg-[#faf9fc]' : 'border-l-transparent hover:bg-[#fafafa]'
                  )}
                >
                  <div className="flex items-center justify-between gap-2">
                    <p className={cn('truncate text-[13px] font-medium', selectedConversationId === conversation.id ? 'text-[#292929]' : 'text-[#5e5e5e]')}>{conversation.title}</p>
                    <span className="shrink-0 rounded-full bg-[#f0f0f0] px-1.5 py-0.5 text-[10px] font-medium text-[#8e8e8e]">{providerLabel(conversation.provider)}</span>
                  </div>
                  <p className="mt-0.5 line-clamp-2 text-xs leading-snug text-[#8e8e8e]">{conversation.lastMessageText}</p>
                  <p className="mt-1 text-[11px] text-[#a0a0a0]">
                    {conversation.unreadCount > 0 && <span className="font-medium text-[#6c00a2]">{conversation.unreadCount} новое</span>}
                    {conversation.unreadCount > 0 && ' · '}
                    {formatTime(new Date(conversation.updatedAt))}
                  </p>
                </button>
              ))}
            </div>
          ) : (
            <div className="flex-1 p-4">
              <EmptyState title="Нет диалогов" />
            </div>
          )}
        </aside>

        <main className="flex min-w-0 flex-1 flex-col">
          {detail.data ? (
            <>
              <header className="shrink-0 border-b border-[#ebebeb] px-4 py-2">
                <div className="flex items-center justify-between gap-2">
                  <div className="min-w-0">
                    <h2 className="truncate text-base font-semibold text-[#292929]">{detail.data.conversation.title}</h2>
                  </div>
                  <div className="shrink-0 rounded-full bg-[#f3f3f3] px-2.5 py-1 text-[11px] font-medium text-[#6e6e6e]">
                    {providerLabel(detail.data.conversation.provider)}
                  </div>
                </div>
              </header>

              <div className="shrink-0 border-b border-[#ebebeb] bg-[#faf9fc] px-4 py-2">
                <p className="text-xs text-[#8e8e8e]">
                  {latestActionableBooking
                    ? `${latestActionableBooking.status === 'pending' ? 'Ожидающая запись' : 'Подтвержденная запись'} · ${new Date(latestActionableBooking.startsAt).toLocaleString('ru', { dateStyle: 'medium', timeStyle: 'short' })}`
                    : 'Новая запись из диалога'}
                </p>
                <div className="mt-2 grid gap-2">
                  <div className="grid gap-2 sm:grid-cols-[56px_minmax(0,1fr)_minmax(0,1fr)_auto] sm:items-end">
                    <span className="text-xs text-[#6b6b6b]">Время</span>
                    <input
                      type="datetime-local"
                      data-testid="booking-start"
                      value={bookingStartDraft}
                      onChange={(event) => {
                        setSelectedSlotId('')
                        setBookingStartDraft(event.target.value)
                      }}
                      placeholder="Начало"
                      className="rounded-[10px] border border-[#e3deea] bg-white px-2.5 py-1.5 text-sm text-[#292929] outline-none focus:border-[#8089a8]"
                    />
                    <input
                      type="datetime-local"
                      data-testid="booking-end"
                      value={bookingEndDraft}
                      onChange={(event) => {
                        setSelectedSlotId('')
                        setBookingEndDraft(event.target.value)
                      }}
                      placeholder="Конец"
                      className="rounded-[10px] border border-[#e3deea] bg-white px-2.5 py-1.5 text-sm text-[#292929] outline-none focus:border-[#8089a8]"
                    />
                    {latestActionableBooking ? (
                      <ActionButton data-testid="booking-cancel" variant="danger" isLoading={isPending(`cancel-booking-${latestActionableBooking.id}`)} loadingLabel="..." onClick={() => void onCancelLatestBooking()}>
                        Отменить
                      </ActionButton>
                    ) : (
                      <span />
                    )}
                  </div>
                  <div className="grid gap-2 sm:grid-cols-[56px_120px_auto_auto] sm:items-end">
                    <span className="text-xs text-[#6b6b6b]">Сумма</span>
                    <input
                      data-testid="booking-amount"
                      value={bookingAmountDraft}
                      onChange={(event) => setBookingAmountDraft(event.target.value)}
                      inputMode="numeric"
                      placeholder="4500"
                      className="rounded-[10px] border border-[#e3deea] bg-white px-2.5 py-1.5 text-sm text-[#292929] outline-none focus:border-[#8089a8]"
                    />
                    <ActionButton
                      data-testid="booking-submit"
                      variant="primary"
                      isLoading={isPending(latestActionableBooking ? `reschedule-confirmed-booking-${latestActionableBooking.id}` : 'create-confirmed-booking')}
                      loadingLabel="..."
                      onClick={() => void onSubmitBooking()}
                    >
                      {latestActionableBooking ? (latestActionableBooking.status === 'pending' ? 'Подтвердить запись' : 'Перенести') : 'Подтвердить запись'}
                    </ActionButton>
                    {latestConfirmedBooking ? (
                      <ActionButton variant="secondary" isLoading={isPending(`complete-booking-${latestConfirmedBooking.id}`)} loadingLabel="..." onClick={() => void onCompleteLatestBooking()}>
                        Завершить
                      </ActionButton>
                    ) : (
                      <span />
                    )}
                  </div>
                </div>
              </div>

              <div className="flex-1 space-y-4 overflow-y-auto p-4">
                {detail.data.messages.map((message) => (
                  <div key={message.id} className={cn('flex max-w-[80%] flex-col', message.direction === 'outbound' ? 'ml-auto items-end' : 'items-start')}>
                    <div className={cn('rounded-2xl px-3.5 py-2.5 text-sm leading-relaxed', message.direction === 'outbound' ? 'rounded-br-md border border-[#e5e0ed] bg-[#f5f2f9] text-[#3d3852]' : 'rounded-bl-md border border-[#e8e8e8] bg-[#f8f8f8] text-[#292929]')}>
                      <p className="whitespace-pre-wrap">{message.text}</p>
                    </div>
                    <span className={cn('mt-0.5 text-[10px]', message.direction === 'outbound' ? 'text-[#8e8e8e]' : 'text-[#a0a0a0]')}>{formatTime(new Date(message.createdAt))}</span>
                  </div>
                ))}
              </div>

              <div className="shrink-0 border-t border-[#ebebeb] p-3">
                <div className="flex gap-2 rounded-xl border border-[#ebebeb] bg-[#fafafa] p-2 transition-colors focus-within:border-[#8089a8] focus-within:bg-white">
                  <textarea
                    value={composeDraft}
                    onChange={(event) => setComposeDraft(event.target.value)}
                    className="min-h-[40px] max-h-32 flex-1 resize-none rounded-lg bg-transparent px-3 py-2 text-sm text-[#292929] outline-none placeholder:text-[#a0a0a0]"
                    placeholder="Написать сообщение..."
                    rows={1}
                  />
                  <div className="relative flex shrink-0 items-end gap-1">
                    <button
                      type="button"
                      onClick={() => setShowReplyStack((value) => !value)}
                      className="relative h-10 w-11 shrink-0"
                      aria-label="Быстрые ответы"
                    >
                      <span className="absolute left-2 top-2 h-7 w-7 rounded-[10px] border border-[#d8d2e3] bg-white shadow-sm" />
                      <span className="absolute left-1.5 top-1.5 h-7 w-7 rounded-[10px] border border-[#d8d2e3] bg-[#f4eff9] shadow-sm" />
                      <span className="absolute left-1 top-1 h-7 w-7 rounded-[10px] border border-[#cabee0] bg-[#ece3f7] shadow-sm" />
                    </button>
                    {showReplyStack ? (
                      <div className="absolute bottom-12 right-12 z-20 w-72 rounded-[14px] border border-[#e6deef] bg-white p-2 shadow-xl">
                        <p className="px-2 pb-1 text-[11px] font-medium uppercase tracking-[0.08em] text-[#8e8e8e]">Быстрые ответы</p>
                        <div className="space-y-1">
                          {quickReplies.map((template, index) => (
                            <button
                              key={index}
                              type="button"
                              onClick={() => onUseTemplate(template)}
                              className="w-full rounded-[10px] border border-transparent bg-[#faf8fc] px-3 py-2 text-left text-xs leading-relaxed text-[#3b3550] transition hover:border-[#ddd3ea] hover:bg-[#f4eff9]"
                            >
                              {template}
                            </button>
                          ))}
                        </div>
                      </div>
                    ) : null}
                    <ActionButton variant="primary" isLoading={isPending('reply')} isDisabled={!selectedConversationId || !composeDraft.trim()} loadingLabel="..." onClick={() => void onReply()}>Отправить</ActionButton>
                  </div>
                </div>
              </div>
            </>
          ) : (
            <div className="flex flex-1 items-center justify-center p-8">
              <EmptyState title="Выберите диалог" description="Откройте чат в списке слева" />
            </div>
          )}
        </main>

        {detail.data ? (
          <aside className="hidden w-[340px] shrink-0 flex-col overflow-y-auto border-l border-[#ebebeb] p-3 lg:flex">
            <div className="rounded-md border border-[#ebebeb] bg-[#f7f7f7] p-3 text-sm">
              <div>
                <span className="mb-1 block text-[11px] font-medium uppercase tracking-[0.08em] text-[#8e8e8e]">Имя клиента</span>
                <div className="flex gap-2">
                  <input
                    value={customerNameDraft}
                    onChange={(event) => setCustomerNameDraft(event.target.value)}
                    className="min-w-0 flex-1 rounded-[10px] border border-[#e3e3e3] bg-white px-3 py-2 font-semibold text-[#292929] outline-none focus:border-[#8089a8]"
                  />
                  <ActionButton
                    type="button"
                    variant="secondary"
                    isLoading={isPending(`customer-name-${detail.data.customer.id}`)}
                    isDisabled={!customerNameDraft.trim() || customerNameDraft.trim() === detail.data.customer.name}
                    loadingLabel="..."
                    onClick={(e) => {
                      e.preventDefault()
                      e.stopPropagation()
                      void onSaveCustomerName()
                    }}
                  >
                    Сохранить
                  </ActionButton>
                </div>
              </div>
              <p className="mt-0.5 text-[#5e5e5e]">{detail.data.customer.phone}</p>
              <p className="text-[#8e8e8e]">{detail.data.customer.email || '—'}</p>
              <p className="mt-2 text-[#5e5e5e]">{detail.data.customer.notes}</p>
              <div className="mt-2 flex gap-1.5">
                <span className="rounded bg-[#ebebeb] px-1.5 py-0.5 text-[10px] text-[#5e5e5e]">{detail.data.customer.bookingCount} записей</span>
                <span className="rounded bg-[#ebebeb] px-1.5 py-0.5 text-[10px] text-[#5e5e5e]">{providerLabel(detail.data.customer.preferredChannel)}</span>
              </div>
            </div>

            <div className="mt-3 rounded-md border border-[#ebebeb] bg-[#f7f7f7] p-3">
              <div className="mb-2 flex items-center justify-between">
                <p className="text-xs font-medium text-[#8e8e8e]">Реальные слоты</p>
                <ActionButton variant="quiet" onClick={() => void router.navigate({ to: '/slots' })}>Открыть слоты</ActionButton>
              </div>
              {Object.keys(groupedVisibleSlots).length ? (
                <div className="space-y-3">
                  {Object.entries(groupedVisibleSlots).map(([slotDate, slots]) => (
                    <div key={slotDate}>
                      <p className="mb-2 text-xs text-[#8e8e8e]">{new Date(`${slotDate}T00:00:00`).toLocaleDateString('ru', { weekday: 'long', day: 'numeric', month: 'long' })}</p>
                      <div className="grid gap-2">
                        {slots.map((slot) => (
                          <button
                            key={slot.id}
                            type="button"
                            onClick={() => onApplyAvailableSlot(slot)}
                            className={cn('cursor-pointer rounded-[10px] border px-3 py-2 text-left text-sm font-medium text-[#292929] transition-colors hover:opacity-90', selectedSlotId === slot.id ? 'ring-1 ring-[#292929]' : '')}
                            style={{ borderColor: slot.colorHex, backgroundColor: `${slot.colorHex}1A` }}
                          >
                            <div className="pointer-events-none flex items-center justify-between gap-2">
                              <span>
                                {new Date(slot.startsAt).toLocaleTimeString('ru', { hour: '2-digit', minute: '2-digit' })} – {new Date(slot.endsAt).toLocaleTimeString('ru', { hour: '2-digit', minute: '2-digit' })}
                              </span>
                              <span className="text-[11px] text-[#5e5e5e]">{slot.colorName}</span>
                            </div>
                          </button>
                        ))}
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <EmptyState title="Нет свободных слотов" description="Добавьте или разблокируйте точки времени в разделе слотов." />
              )}
            </div>

            {showHistory ? (
              <div className="mt-3">
                <p className="mb-2 text-xs text-[#8e8e8e]">История</p>
                <div className="space-y-1.5">
                  {customerBookings.slice(0, 5).map((booking) => (
                      <div key={booking.id} className="rounded border border-[#ebebeb] bg-white px-2 py-1.5 text-xs">
                      <div className="flex justify-between">
                        <span>{new Date(booking.startsAt).toLocaleString()}</span>
                        <span className="text-[#8e8e8e]">{bookingStatusLabel(booking.status)}</span>
                      </div>
                      {booking.amount > 0 ? <p className="mt-0.5 text-[#5e5e5e]">{booking.amount} ₽</p> : null}
                      <p className="mt-0.5 text-[#8e8e8e]">{booking.notes || '—'}</p>
                    </div>
                  ))}
                </div>
              </div>
            ) : null}

            <ActionButton variant="quiet" onClick={() => setShowHistory((value) => !value)} className="mt-2 w-full">
              {showHistory ? 'Скрыть историю' : 'История клиента'}
            </ActionButton>
          </aside>
        ) : null}
      </div>
    </section>
  )
}
