import { useMemo } from 'react'
import { useRouter } from '@tanstack/react-router'
import { ActionButton } from '../components/ActionButton'
import { EmptyState } from '../components/EmptyState'
import { ListItemCard } from '../components/ListItemCard'
import { MetricTile } from '../components/MetricTile'
import { PageHeader } from '../components/PageHeader'
import { Section } from '../components/Section'
import { Badge } from '../components/ui/badge'
import { useActionRunner } from '../lib/actions'
import { useAnalytics, useAvailableSlots, useBookings, useConversations, useSimulateWebhookMutation } from '../lib/queries'
import type { Booking, Conversation, DailySlot } from '../lib/types'
import { useUIStore } from '../stores/ui'

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

function startOfDay(value: Date) {
  return new Date(value.getFullYear(), value.getMonth(), value.getDate())
}

function startOfWeekMonday(value: Date) {
  const base = startOfDay(value)
  const weekday = (base.getDay() + 6) % 7
  base.setDate(base.getDate() - weekday)
  return base
}

function startOfMonth(value: Date) {
  return new Date(value.getFullYear(), value.getMonth(), 1)
}

function addDays(value: Date, days: number) {
  const next = new Date(value)
  next.setDate(next.getDate() + days)
  return next
}

function inRange(value: Date, from: Date, to: Date) {
  return value >= from && value < to
}

function formatCurrency(value: number) {
  return new Intl.NumberFormat('ru-RU', { style: 'currency', currency: 'RUB', maximumFractionDigits: 0 }).format(value)
}

function formatDelta(current: number, previous: number) {
  const difference = current - previous
  if (difference === 0) {
    return 'без изменений'
  }
  const direction = difference > 0 ? 'рост' : 'снижение'
  return `${direction} ${formatCurrency(Math.abs(difference))}`
}

function openInboxForConversation(router: ReturnType<typeof useRouter>, setSelectedConversationId: (id: string | null) => void, conversationId: string) {
  setSelectedConversationId(conversationId)
  void router.navigate({ to: '/dialogs' })
}

function bookingDate(item: Booking) {
  return new Date(item.startsAt)
}

function slotDate(item: DailySlot) {
  return new Date(item.startsAt)
}

function conversationDate(item: Conversation) {
  return new Date(item.updatedAt)
}

export function DashboardPage() {
  const router = useRouter()
  const conversations = useConversations()
  const bookings = useBookings('all')
  const analytics = useAnalytics()
  const availableSlots = useAvailableSlots(todayLocal(), addDaysLocal(14))
  const simulateWebhook = useSimulateWebhookMutation()
  const setSelectedConversationId = useUIStore((state) => state.setSelectedConversationId)
  const setDialogFilter = useUIStore((state) => state.setDialogFilter)
  const { runAction, isPending } = useActionRunner()

  const conversationItems = conversations.data?.items ?? []
  const bookingItems = bookings.data?.items ?? []
  const slotItems = availableSlots.data?.items ?? []

  const stats = useMemo(() => {
    const now = new Date()
    const dayStart = startOfDay(now)
    const dayEnd = addDays(dayStart, 1)
    const prevDayStart = addDays(dayStart, -1)
    const prevDayEnd = dayStart
    const weekStart = startOfWeekMonday(now)
    const weekEnd = addDays(weekStart, 7)
    const prevWeekStart = addDays(weekStart, -7)
    const prevWeekEnd = weekStart
    const monthStart = startOfMonth(now)
    const monthEnd = new Date(now.getFullYear(), now.getMonth() + 1, 1)
    const prevMonthStart = new Date(now.getFullYear(), now.getMonth() - 1, 1)
    const prevMonthEnd = monthStart
    const monthAgo = addDays(dayStart, -30)

    const paidStatuses = new Set<Booking['status']>(['confirmed', 'completed'])

    const todayBookings = bookingItems.filter((item) => item.status !== 'cancelled' && inRange(bookingDate(item), dayStart, dayEnd))
    const todayTimeline = [...todayBookings].sort((a, b) => bookingDate(a).getTime() - bookingDate(b).getTime())
    const upcomingToday = todayBookings
      .filter((item) => bookingDate(item) >= now)
      .sort((a, b) => bookingDate(a).getTime() - bookingDate(b).getTime())
    const todayCancelled = bookingItems.filter((item) => item.status === 'cancelled' && inRange(bookingDate(item), dayStart, dayEnd)).length

    const freeToday = slotItems.filter((slot) => slot.slotDate === todayLocal() && slotDate(slot) >= now)
    const upcomingFreeSlots = slotItems
      .filter((slot) => slotDate(slot) >= now)
      .sort((a, b) => slotDate(a).getTime() - slotDate(b).getTime())
    const nextFreeSlots = upcomingFreeSlots.slice(0, 6)

    const waitingConversations = conversationItems.filter((conversation) => conversation.status !== 'closed' && conversation.unreadCount > 0)
    const unreadMessages = waitingConversations.reduce((acc, item) => acc + item.unreadCount, 0)
    const waitingCustomers = new Set(waitingConversations.map((item) => item.customerId)).size

    const todayRevenue = bookingItems
      .filter((item) => paidStatuses.has(item.status) && inRange(bookingDate(item), dayStart, dayEnd))
      .reduce((acc, item) => acc + item.amount, 0)
    const weekRevenue = bookingItems
      .filter((item) => paidStatuses.has(item.status) && inRange(bookingDate(item), weekStart, weekEnd))
      .reduce((acc, item) => acc + item.amount, 0)
    const monthRevenue = bookingItems
      .filter((item) => paidStatuses.has(item.status) && inRange(bookingDate(item), monthStart, monthEnd))
      .reduce((acc, item) => acc + item.amount, 0)
    const prevDayRevenue = bookingItems
      .filter((item) => paidStatuses.has(item.status) && inRange(bookingDate(item), prevDayStart, prevDayEnd))
      .reduce((acc, item) => acc + item.amount, 0)
    const prevWeekRevenue = bookingItems
      .filter((item) => paidStatuses.has(item.status) && inRange(bookingDate(item), prevWeekStart, prevWeekEnd))
      .reduce((acc, item) => acc + item.amount, 0)
    const prevMonthRevenue = bookingItems
      .filter((item) => paidStatuses.has(item.status) && inRange(bookingDate(item), prevMonthStart, prevMonthEnd))
      .reduce((acc, item) => acc + item.amount, 0)

    const visitsByCustomer = new Map<string, { name: string; dates: Date[] }>()
    for (const booking of bookingItems) {
      if (!paidStatuses.has(booking.status)) continue
      const item = visitsByCustomer.get(booking.customerId) ?? { name: booking.customerName || 'Клиент', dates: [] }
      item.dates.push(bookingDate(booking))
      visitsByCustomer.set(booking.customerId, item)
    }
    const repeatCustomers = [...visitsByCustomer.values()].filter((item) => item.dates.length >= 2)
    const longAbsentCustomers = repeatCustomers
      .map((item) => ({ ...item, lastVisit: item.dates.sort((a, b) => b.getTime() - a.getTime())[0] }))
      .filter((item) => item.lastVisit < monthAgo)
      .sort((a, b) => a.lastVisit.getTime() - b.lastVisit.getTime())

    const cancelledWeek = bookingItems.filter((item) => item.status === 'cancelled' && inRange(bookingDate(item), weekStart, weekEnd)).length
    const totalWeek = bookingItems.filter((item) => inRange(bookingDate(item), weekStart, weekEnd)).length
    const noShowRate = analytics.data?.noShowRate ?? (totalWeek ? Math.round((cancelledWeek / totalWeek) * 100) : 0)
    const noShowEstimate = Math.round((totalWeek * noShowRate) / 100)

    const inquiriesWeek = conversationItems.filter((item) => inRange(conversationDate(item), weekStart, weekEnd)).length
    const bookedFromInquiriesWeek = bookingItems.filter((item) => item.status !== 'cancelled' && inRange(bookingDate(item), weekStart, weekEnd)).length
    const conversionWeek = inquiriesWeek > 0 ? Math.round((bookedFromInquiriesWeek / inquiriesWeek) * 100) : 0

    return {
      dayStart,
      todayBookings,
      todayTimeline,
      upcomingToday,
      todayCancelled,
      freeToday,
      nextFreeSlots,
      upcomingFreeSlots,
      waitingConversations,
      unreadMessages,
      waitingCustomers,
      todayRevenue,
      weekRevenue,
      monthRevenue,
      prevDayRevenue,
      prevWeekRevenue,
      prevMonthRevenue,
      repeatCustomers,
      longAbsentCustomers,
      cancelledWeek,
      noShowRate,
      noShowEstimate,
      inquiriesWeek,
      bookedFromInquiriesWeek,
      conversionWeek
    }
  }, [analytics.data?.noShowRate, bookingItems, conversationItems, slotItems])

  const onRefresh = async () => {
    await runAction({
      key: 'dashboard-refresh',
      event: 'dashboard_refresh',
      execute: async () => ({ ok: true }),
      successMessage: 'Данные дашборда обновлены.',
      invalidateKeys: [['conversations'], ['bookings', 'all'], ['available-slots'], ['analytics'], ['dashboard']],
      telemetry: { screen: 'dashboard' }
    })
  }

  const onCreateTestLead = async () => {
    await runAction({
      key: 'dashboard-create-lead',
      event: 'dashboard_create_test_lead',
      execute: () => simulateWebhook.mutateAsync({ provider: 'telegram', customerName: 'Лид с дашборда', text: 'Нужна запись на ближайшее свободное окно' }),
      successMessage: 'Тестовый лид создан с дашборда.',
      invalidateKeys: [['conversations'], ['dashboard']],
      telemetry: { screen: 'dashboard' }
    })
  }

  const openInbox = async () => {
    setDialogFilter('all')
    const first = stats.waitingConversations[0] ?? conversationItems[0]
    if (first) {
      setSelectedConversationId(first.id)
    }
    await router.navigate({ to: '/dialogs' })
  }

  return (
    <section className="space-y-6">
      <PageHeader title="Дашборд" actions={<ActionButton variant="quiet" isLoading={isPending('dashboard-refresh') || conversations.isFetching || bookings.isFetching || availableSlots.isFetching || analytics.isFetching} loadingLabel="..." onClick={() => void onRefresh()}>Обновить</ActionButton>} />

      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <MetricTile accent="violet" label="Записи на сегодня" value={stats.todayBookings.length} hint={`${stats.upcomingToday.length} скоро · ${stats.todayCancelled} отмен · ${stats.freeToday.length} окон`} onClick={() => void router.navigate({ to: '/slots' })} />
        <MetricTile accent="blue" label="Новые обращения" value={stats.unreadMessages} hint={`${stats.waitingConversations.length} диалогов · ${stats.waitingCustomers} клиентов`} onClick={() => void openInbox()} />
        <MetricTile accent="green" label="Доход (день / неделя / месяц)" value={formatCurrency(stats.todayRevenue)} hint={`${formatCurrency(stats.weekRevenue)} · ${formatCurrency(stats.monthRevenue)}`} onClick={() => void router.navigate({ to: '/analytics' })} />
        <MetricTile accent="amber" label="Повторные клиенты" value={stats.repeatCustomers.length} hint={stats.longAbsentCustomers.length ? `${stats.longAbsentCustomers.length} давно не приходили` : 'все активны'} onClick={() => void router.navigate({ to: '/dialogs' })} />
        <MetricTile accent="violet" label="Свободные окна" value={stats.upcomingFreeSlots.length} hint={stats.nextFreeSlots[0] ? `ближайшее: ${new Date(stats.nextFreeSlots[0].startsAt).toLocaleString('ru', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })}` : 'нет доступных окон'} onClick={() => void router.navigate({ to: '/slots' })} />
        <MetricTile accent="amber" label="Отмены и неявки (неделя)" value={`${stats.cancelledWeek} / ${stats.noShowEstimate}`} hint={`отмены / неявки · no-show ${stats.noShowRate}%`} onClick={() => void router.navigate({ to: '/analytics' })} />
        <MetricTile accent="blue" label="Конверсия обращение → запись" value={`${stats.conversionWeek}%`} hint={`${stats.inquiriesWeek} обращений → ${stats.bookedFromInquiriesWeek} записей`} onClick={() => void router.navigate({ to: '/analytics' })} />
      </div>

      <div className="grid gap-6 xl:grid-cols-2">
        <Section title="1. Записи на сегодня" action={<button type="button" onClick={() => void router.navigate({ to: '/slots' })} className="text-xs font-medium text-[#7c3aed] hover:text-[#6d28d9] hover:underline">Открыть</button>}>
          {stats.todayTimeline.length ? (
            <div className="space-y-2">
              {stats.todayTimeline.slice(0, 5).map((booking) => (
                <ListItemCard
                  key={booking.id}
                  title={booking.customerName}
                  subtitle={booking.notes || 'Запись на сегодня'}
                  meta={`${new Date(booking.startsAt).toLocaleString('ru')} · ${new Date(booking.startsAt) >= new Date() ? 'скоро' : 'уже была'}`}
                  actionLabel="В слоты"
                  onClick={() => void router.navigate({ to: '/slots' })}
                />
              ))}
            </div>
          ) : (
            <EmptyState title="Сегодня нет записей" description="" />
          )}
        </Section>

        <Section title="2. Новые обращения" action={<button type="button" onClick={() => void openInbox()} className="text-xs font-medium text-[#7c3aed] hover:text-[#6d28d9] hover:underline">Открыть</button>}>
          {stats.waitingConversations.length ? (
            <div className="space-y-2">
              {stats.waitingConversations.slice(0, 5).map((conversation) => (
                <ListItemCard
                  key={conversation.id}
                  title={conversation.title}
                  subtitle={conversation.lastMessageText}
                  meta={new Date(conversation.updatedAt).toLocaleString('ru')}
                  badge={<Badge variant="secondary">{conversation.unreadCount}</Badge>}
                  actionLabel="Ответить"
                  onClick={() => openInboxForConversation(router, setSelectedConversationId, conversation.id)}
                />
              ))}
            </div>
          ) : (
            <EmptyState title="Нет обращений без ответа" description="" actions={<ActionButton variant="secondary" onClick={() => void onCreateTestLead()}>Тест лид</ActionButton>} />
          )}
        </Section>

        <Section title="3. Доход за сегодня / неделю / месяц" action={<button type="button" onClick={() => void router.navigate({ to: '/analytics' })} className="text-xs font-medium text-[#7c3aed] hover:text-[#6d28d9] hover:underline">Открыть</button>}>
          <div className="grid gap-3 md:grid-cols-3">
            <div className="relative overflow-hidden rounded-md border border-emerald-200 bg-emerald-50/80 p-3">
              <span className="absolute right-2 top-2 h-8 w-8 rounded-full bg-emerald-200/50" aria-hidden />
              <p className="text-xs font-medium text-emerald-700">Сегодня</p>
              <p className="mt-1 text-lg font-semibold text-emerald-900">{formatCurrency(stats.todayRevenue)}</p>
              <p className="text-xs text-emerald-600">{formatDelta(stats.todayRevenue, stats.prevDayRevenue)}</p>
            </div>
            <div className="relative overflow-hidden rounded-md border border-blue-200 bg-blue-50/80 p-3">
              <span className="absolute right-2 top-2 h-8 w-8 rounded-full bg-blue-200/50" aria-hidden />
              <p className="text-xs font-medium text-blue-700">Неделя</p>
              <p className="mt-1 text-lg font-semibold text-blue-900">{formatCurrency(stats.weekRevenue)}</p>
              <p className="text-xs text-blue-600">{formatDelta(stats.weekRevenue, stats.prevWeekRevenue)}</p>
            </div>
            <div className="relative overflow-hidden rounded-md border border-violet-200 bg-violet-50/80 p-3">
              <span className="absolute right-2 top-2 h-8 w-8 rounded-full bg-violet-200/50" aria-hidden />
              <p className="text-xs font-medium text-violet-700">Месяц</p>
              <p className="mt-1 text-lg font-semibold text-violet-900">{formatCurrency(stats.monthRevenue)}</p>
              <p className="text-xs text-violet-600">{formatDelta(stats.monthRevenue, stats.prevMonthRevenue)}</p>
            </div>
          </div>
        </Section>

        <Section title="4. Повторные клиенты" action={<button type="button" onClick={() => void router.navigate({ to: '/dialogs' })} className="text-xs font-medium text-[#7c3aed] hover:text-[#6d28d9] hover:underline">Открыть</button>}>
          {stats.longAbsentCustomers.length ? (
            <div className="space-y-2">
              {stats.longAbsentCustomers.slice(0, 5).map((item) => (
                <ListItemCard
                  key={`${item.name}-${item.lastVisit.toISOString()}`}
                  title={item.name}
                  subtitle="Кого можно вернуть"
                  meta={`Последний визит: ${item.lastVisit.toLocaleDateString('ru')}`}
                  actionLabel="Связаться"
                  onClick={() => void router.navigate({ to: '/dialogs' })}
                />
              ))}
            </div>
          ) : (
            <EmptyState title="Нет клиентов для возврата" description="В пределах последних 30 дней все были активны." />
          )}
        </Section>

        <Section title="5. Свободные окна" action={<button type="button" onClick={() => void router.navigate({ to: '/slots' })} className="text-xs font-medium text-[#7c3aed] hover:text-[#6d28d9] hover:underline">Открыть</button>}>
          {stats.nextFreeSlots.length ? (
            <div className="space-y-2">
              {stats.nextFreeSlots.map((slot) => (
                <ListItemCard
                  key={slot.id}
                  title={new Date(slot.startsAt).toLocaleDateString('ru', { weekday: 'long', day: 'numeric', month: 'long' })}
                  subtitle={`${new Date(slot.startsAt).toLocaleTimeString('ru', { hour: '2-digit', minute: '2-digit' })} – ${new Date(slot.endsAt).toLocaleTimeString('ru', { hour: '2-digit', minute: '2-digit' })}`}
                  actionLabel="Заполнить"
                  onClick={() => void router.navigate({ to: '/slots' })}
                />
              ))}
            </div>
          ) : (
            <EmptyState title="Свободных окон рядом нет" description="" />
          )}
        </Section>
      </div>

      <Section title="6-7. Отмены, неявки и конверсия">
        <div className="grid gap-3 md:grid-cols-2">
          <div className="relative overflow-hidden rounded-md border-l-4 border-l-amber-400 border border-[#ebebeb] bg-amber-50/50 p-3">
            <p className="text-xs font-medium text-amber-700">Отмены и неявки</p>
            <p className="mt-1 text-lg font-semibold text-amber-900">{stats.cancelledWeek} отмен за неделю</p>
            <p className="text-xs text-amber-600">Оценка неявок: {stats.noShowEstimate} · no-show {stats.noShowRate}%</p>
          </div>
          <div className="relative overflow-hidden rounded-md border-l-4 border-l-blue-500 border border-[#ebebeb] bg-blue-50/50 p-3">
            <p className="text-xs font-medium text-blue-700">Конверсия из обращения в запись</p>
            <p className="mt-1 text-lg font-semibold text-blue-900">{stats.conversionWeek}%</p>
            <p className="text-xs text-blue-600">{stats.inquiriesWeek} обращений → {stats.bookedFromInquiriesWeek} записей</p>
          </div>
        </div>
      </Section>
    </section>
  )
}
