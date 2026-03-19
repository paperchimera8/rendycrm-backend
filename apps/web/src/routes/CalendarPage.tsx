import { useEffect, useMemo, useState } from 'react'
import { ActionButton } from '../components/ActionButton'
import { EmptyState } from '../components/EmptyState'
import { SectionCard } from '../components/SectionCard'
import { useBookPublicCalendarMutation, usePublicCalendar } from '../lib/queries'
import type { DailySlot } from '../lib/types'

function formatDate(value: Date) {
  const year = value.getFullYear()
  const month = `${value.getMonth() + 1}`.padStart(2, '0')
  const day = `${value.getDate()}`.padStart(2, '0')
  return `${year}-${month}-${day}`
}

function todayLocal() {
  return formatDate(new Date())
}

function addDays(date: string, days: number) {
  const next = new Date(`${date}T00:00:00`)
  next.setDate(next.getDate() + days)
  return formatDate(next)
}

function formatDayLabel(date: string, timezone: string) {
  return new Date(`${date}T00:00:00`).toLocaleDateString('ru-RU', {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    timeZone: timezone
  })
}

function formatTime(value: string, timezone: string) {
  return new Date(value).toLocaleTimeString('ru-RU', {
    hour: '2-digit',
    minute: '2-digit',
    timeZone: timezone
  })
}

function formatDateTime(value: string, timezone: string) {
  return new Date(value).toLocaleString('ru-RU', {
    day: 'numeric',
    month: 'long',
    hour: '2-digit',
    minute: '2-digit',
    timeZone: timezone
  })
}

function formatSlotLabel(slot: DailySlot, timezone: string) {
  return `${formatTime(slot.startsAt, timezone)} - ${formatTime(slot.endsAt, timezone)}`
}

export function CalendarPage() {
  const token = useMemo(() => new URLSearchParams(window.location.search).get('token')?.trim() ?? '', [])
  const dateFrom = useMemo(() => todayLocal(), [])
  const dateTo = useMemo(() => addDays(dateFrom, 13), [dateFrom])
  const calendar = usePublicCalendar(token, dateFrom, dateTo)
  const bookMutation = useBookPublicCalendarMutation()

  const [selectedSlot, setSelectedSlot] = useState<DailySlot | null>(null)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')

  const timezone = calendar.data?.workspace.timezone || 'Europe/Moscow'

  const groupedSlots = useMemo(() => {
    const buckets = new Map<string, DailySlot[]>()
    for (const item of calendar.data?.items ?? []) {
      const dayItems = buckets.get(item.slotDate) ?? []
      dayItems.push(item)
      buckets.set(item.slotDate, dayItems)
    }
    return Array.from(buckets.entries())
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([date, items]) => ({
        date,
        items: [...items].sort((left, right) => left.startsAt.localeCompare(right.startsAt))
      }))
  }, [calendar.data?.items])

  useEffect(() => {
    if (!selectedSlot) {
      return
    }
    const stillAvailable = (calendar.data?.items ?? []).some((item) => item.id === selectedSlot.id)
    if (!stillAvailable) {
      setSelectedSlot(null)
    }
  }, [calendar.data?.items, selectedSlot])

  async function onBook() {
    if (!selectedSlot || !token) {
      return
    }
    setError('')
    setSuccess('')
    try {
      const response = await bookMutation.mutateAsync({
        token,
        dailySlotId: selectedSlot.id
      })
      setSuccess(`Запись подтверждена на ${formatDateTime(response.booking.startsAt, timezone)}.`)
      setSelectedSlot(null)
      await calendar.refetch()
    } catch (bookingError) {
      setError(bookingError instanceof Error ? bookingError.message : 'Не удалось создать запись.')
      await calendar.refetch()
    }
  }

  return (
    <main className="mx-auto flex min-h-screen w-full max-w-5xl flex-col gap-4 px-4 py-6">
      <SectionCard
        title={calendar.data?.workspace.name || 'Календарь записи'}
        subtitle="Выберите свободный слот. Календарь обновляется автоматически каждые 30 секунд."
        actions={
          <ActionButton
            variant="quiet"
            isLoading={calendar.isFetching}
            loadingLabel="Обновляю..."
            onClick={() => void calendar.refetch()}
          >
            Обновить
          </ActionButton>
        }
      >
        <div className="text-xs text-[#8e8e8e]">
          Запись открывается без входа в кабинет только по ссылке из Telegram.
        </div>
      </SectionCard>

      {!token ? (
        <SectionCard>
          <EmptyState
            title="Ссылка недействительна"
            description="Откройте календарь из кнопки в Telegram, чтобы продолжить запись."
          />
        </SectionCard>
      ) : calendar.isLoading ? (
        <SectionCard title="Загрузка календаря" subtitle="Получаем актуальные свободные слоты.">
          <div className="grid gap-3 md:grid-cols-2">
            {Array.from({ length: 4 }).map((_, index) => (
              <div key={index} className="h-24 animate-pulse rounded-lg border border-[#ebebeb] bg-[#f7f7f7]" />
            ))}
          </div>
        </SectionCard>
      ) : calendar.isError ? (
        <SectionCard>
          <EmptyState
            title="Не удалось открыть календарь"
            description={calendar.error instanceof Error ? calendar.error.message : 'Попробуйте открыть ссылку заново из Telegram.'}
          />
        </SectionCard>
      ) : (
        <>
          <SectionCard
            title="Свободные слоты"
            subtitle={
              groupedSlots.length > 0
                ? `Период: ${formatDayLabel(dateFrom, timezone)} - ${formatDayLabel(dateTo, timezone)}`
                : 'Свободные окна появятся здесь сразу после синхронизации расписания мастера.'
            }
          >
            {groupedSlots.length === 0 ? (
              <EmptyState
                title="Свободных слотов пока нет"
                description="Попробуйте обновить страницу чуть позже."
              />
            ) : (
              <div className="grid gap-4 lg:grid-cols-2">
                {groupedSlots.map((day) => (
                  <div key={day.date} className="rounded-lg border border-[#ebebeb] p-4">
                    <div className="mb-3">
                      <h2 className="text-sm font-semibold capitalize text-[#292929]">{formatDayLabel(day.date, timezone)}</h2>
                      <p className="mt-0.5 text-xs text-[#8e8e8e]">{day.items.length} слотов доступно</p>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      {day.items.map((slot) => {
                        const active = selectedSlot?.id === slot.id
                        return (
                          <button
                            key={slot.id}
                            type="button"
                            onClick={() => {
                              setError('')
                              setSuccess('')
                              setSelectedSlot(slot)
                            }}
                            className={`rounded-full border px-3 py-2 text-sm transition-colors ${
                              active
                                ? 'border-[#7c3aed] bg-[#7c3aed] text-white'
                                : 'border-[#ebebeb] bg-white text-[#292929] hover:border-[#a78bfa] hover:bg-[#faf5ff]'
                            }`}
                          >
                            {formatSlotLabel(slot, timezone)}
                          </button>
                        )
                      })}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </SectionCard>

          <SectionCard
            title="Подтверждение"
            subtitle={selectedSlot ? 'Проверьте выбранный слот и подтвердите запись.' : 'Выберите слот выше.'}
            actions={
              selectedSlot ? (
                <ActionButton
                  variant="primary"
                  isLoading={bookMutation.isPending}
                  loadingLabel="Подтверждаю..."
                  onClick={() => void onBook()}
                >
                  Подтвердить запись
                </ActionButton>
              ) : undefined
            }
          >
            {selectedSlot ? (
              <div className="space-y-2 text-sm">
                <p className="font-medium text-[#292929]">{formatDayLabel(selectedSlot.slotDate, timezone)}</p>
                <p className="text-[#5e5e5e]">{formatSlotLabel(selectedSlot, timezone)}</p>
              </div>
            ) : (
              <EmptyState title="Слот не выбран" description="Нажмите на свободное время в календаре." />
            )}

            {error ? <p className="mt-3 text-sm text-red-600">{error}</p> : null}
            {success ? <p className="mt-3 text-sm text-emerald-600">{success}</p> : null}
          </SectionCard>
        </>
      )}
    </main>
  )
}
