import { useEffect, useMemo, useState } from 'react'
import { useRouter } from '@tanstack/react-router'
import { ActionButton } from '../components/ActionButton'
import { EmptyState } from '../components/EmptyState'
import { PageHeader } from '../components/PageHeader'
import { SectionCard } from '../components/SectionCard'
import { useActionRunner } from '../lib/actions'
import { slotMutationInvalidateKeys } from '../lib/cache'
import {
  useCreateDaySlotMutation,
  useDeleteDaySlotMutation,
  useMoveDaySlotMutation,
  useSlotEditor,
  useUpdateDaySlotMutation,
  useWeekSlots
} from '../lib/queries'
import type { DailySlot, SlotColorPreset, WeekSlotDay } from '../lib/types'

type SlotDraft = {
  startTime: string
  endTime: string
  note: string
  colorPresetId: string
}

function todayLocal() {
  const now = new Date()
  const year = now.getFullYear()
  const month = `${now.getMonth() + 1}`.padStart(2, '0')
  const day = `${now.getDate()}`.padStart(2, '0')
  return `${year}-${month}-${day}`
}

function timeInputFromDate(value: string) {
  const date = new Date(value)
  const hours = `${date.getHours()}`.padStart(2, '0')
  const minutes = `${date.getMinutes()}`.padStart(2, '0')
  return `${hours}:${minutes}`
}

function minutesFromTimeInput(value: string) {
  const [hours, minutes] = value.split(':').map(Number)
  return hours * 60 + minutes
}

function addMinutesToTimeInput(value: string, deltaMinutes: number) {
  const total = minutesFromTimeInput(value) + deltaMinutes
  const normalized = ((total % (24 * 60)) + 24 * 60) % (24 * 60)
  const hours = `${Math.floor(normalized / 60)}`.padStart(2, '0')
  const minutes = `${normalized % 60}`.padStart(2, '0')
  return `${hours}:${minutes}`
}

function durationFromTimeRange(startValue: string, endValue: string) {
  return minutesFromTimeInput(endValue) - minutesFromTimeInput(startValue)
}

function durationLabel(durationMinutes: number) {
  return `${durationMinutes} мин`
}

function isDemoText(value: string) {
  return /новый лид|demo flow/i.test(value.trim())
}

function visibleSlotText(value: string) {
  return isDemoText(value) ? '' : value
}

function combineDateAndTime(date: string, time: string) {
  return new Date(`${date}T${time}:00`).toISOString()
}

function formatDateLabel(date: string) {
  return new Date(`${date}T00:00:00`).toLocaleDateString('ru', { day: 'numeric', month: 'short' })
}

function reorderByIndex<T extends { id: string }>(items: T[], movedId: string, targetIndex: number) {
  const from = items.findIndex((item) => item.id === movedId)
  if (from === -1) return items
  const next = [...items]
  const [item] = next.splice(from, 1)
  const boundedIndex = Math.max(0, Math.min(targetIndex, next.length))
  next.splice(boundedIndex, 0, item)
  return next
}

function buildDrafts(days: WeekSlotDay[], colors: SlotColorPreset[], defaultDurationMinutes: number) {
  const primaryColorId = colors[0]?.id || ''
  return Object.fromEntries(
    days.map((day) => [
      day.date,
      {
        startTime: '10:00',
        endTime: addMinutesToTimeInput('10:00', defaultDurationMinutes || 60),
        note: '',
        colorPresetId: primaryColorId
      }
    ])
  ) as Record<string, SlotDraft>
}

function normalizeWeekDays(days: WeekSlotDay[]) {
  return days.map((day) => ({
    ...day,
    slots: Array.isArray(day.slots) ? day.slots : []
  }))
}

function renderColorSwatches(colors: SlotColorPreset[], selectedId: string, onSelect: (id: string) => void, disabled = false) {
  return (
    <div className="flex flex-wrap gap-1.5">
      {colors.map((color) => {
        const active = selectedId === color.id
        return (
          <button
            key={color.id}
            type="button"
            title={color.name}
            disabled={disabled}
            onClick={() => onSelect(color.id)}
            className={`h-5 w-5 rounded-full border transition ${active ? 'border-[#292929] ring-1 ring-[#292929]' : 'border-white ring-1 ring-[#d6d6d6]'} ${disabled ? 'cursor-not-allowed opacity-50' : ''}`}
            style={{ backgroundColor: color.hex }}
          />
        )
      })}
    </div>
  )
}

function isLockedSlot(slot: DailySlot) {
  return slot.status === 'held' || slot.status === 'booked'
}

export function AvailabilityPage() {
  const router = useRouter()
  const baseDate = todayLocal()
  const editor = useSlotEditor(baseDate)
  const week = useWeekSlots(baseDate)
  const createDaySlotMutation = useCreateDaySlotMutation()
  const updateDaySlotMutation = useUpdateDaySlotMutation()
  const moveDaySlotMutation = useMoveDaySlotMutation()
  const deleteDaySlotMutation = useDeleteDaySlotMutation()
  const { runAction, isPending } = useActionRunner()

  const [colorsDraft, setColorsDraft] = useState<SlotColorPreset[]>([])
  const [daysDraft, setDaysDraft] = useState<WeekSlotDay[]>([])
  const [draftsByDate, setDraftsByDate] = useState<Record<string, SlotDraft>>({})
  const [openCreatorDate, setOpenCreatorDate] = useState<string | null>(null)
  const [dragSlot, setDragSlot] = useState<{ id: string; slotDate: string } | null>(null)

  useEffect(() => {
    if (!editor.data) return
    setColorsDraft(editor.data.colors)
  }, [editor.data])

  useEffect(() => {
    if (!week.data || !editor.data) return
    const normalizedDays = normalizeWeekDays(week.data.days)
    setDaysDraft(normalizedDays)
    setDraftsByDate(buildDrafts(normalizedDays, editor.data.colors ?? [], editor.data.settings?.defaultDurationMinutes || 60))
  }, [week.data, editor.data])

  const onCreateDaySlot = async (slotDate: string, colorPresetId?: string) => {
    const draft = draftsByDate[slotDate]
    if (!draft) return
    await runAction({
      key: `day-slot-create-${slotDate}`,
      event: 'day_slot_created',
      execute: () => {
        const durationMinutes = durationFromTimeRange(draft.startTime, draft.endTime)
        if (durationMinutes <= 0) {
          throw new Error('Время окончания должно быть позже времени начала.')
        }
        return createDaySlotMutation.mutateAsync({
          slotDate,
          startsAt: combineDateAndTime(slotDate, draft.startTime),
          durationMinutes,
          colorPresetId: colorPresetId || draft.colorPresetId || colorsDraft[0]?.id || '',
          status: 'free',
          note: draft.note.trim()
        })
      },
      successMessage: 'Слот добавлен.',
      invalidateKeys: slotMutationInvalidateKeys(),
      telemetry: { screen: 'slots', slotDate },
      onSuccess: () => {
        setOpenCreatorDate(null)
        setDraftsByDate((state) => ({
          ...state,
          [slotDate]: {
            ...state[slotDate],
            note: ''
          }
        }))
      }
    })
  }

  const onSaveDaySlot = async (slot: DailySlot) => {
    await runAction({
      key: `day-slot-save-${slot.id}`,
      event: 'day_slot_saved',
      execute: () =>
        updateDaySlotMutation.mutateAsync({
          id: slot.id,
          slotDate: slot.slotDate,
          startsAt: slot.startsAt,
          durationMinutes: slot.durationMinutes,
          colorPresetId: slot.colorPresetId,
          note: slot.note
        }),
      successMessage: 'Слот сохранен.',
      invalidateKeys: slotMutationInvalidateKeys(),
      telemetry: { screen: 'slots', entityId: slot.id }
    })
  }

  const onChangeSlotColor = async (slot: DailySlot, colorPresetId: string) => {
    const previousDays = daysDraft
    const nextSlot = { ...slot, colorPresetId }
    setDaysDraft((state) =>
      state.map((day) => ({
        ...day,
        slots: day.slots.map((item) => (item.id === slot.id ? { ...item, colorPresetId, colorHex: colorsDraft.find((color) => color.id === colorPresetId)?.hex || item.colorHex } : item))
      }))
    )
    const result = await runAction({
      key: `day-slot-color-${slot.id}`,
      event: 'day_slot_color_changed',
      execute: () =>
        updateDaySlotMutation.mutateAsync({
          id: nextSlot.id,
          slotDate: nextSlot.slotDate,
          startsAt: nextSlot.startsAt,
          durationMinutes: nextSlot.durationMinutes,
          colorPresetId: nextSlot.colorPresetId,
          note: nextSlot.note
        }),
      successMessage: 'Цвет слота обновлен.',
      invalidateKeys: slotMutationInvalidateKeys(),
      telemetry: { screen: 'slots', entityId: slot.id }
    })
    if (!result) {
      setDaysDraft(previousDays)
    }
  }

  const onDeleteDaySlot = async (slot: DailySlot) => {
    await runAction({
      key: `day-slot-delete-${slot.id}`,
      event: 'day_slot_deleted',
      execute: () => deleteDaySlotMutation.mutateAsync(slot.id),
      successMessage: 'Слот удален.',
      invalidateKeys: slotMutationInvalidateKeys(),
      telemetry: { screen: 'slots', entityId: slot.id }
    })
  }

  const onMoveDaySlot = async (slot: DailySlot, targetSlotDate: string, targetIndex: number) => {
    const previousDays = daysDraft
    const movedStartsAt = combineDateAndTime(targetSlotDate, timeInputFromDate(slot.startsAt))
    const movedEndsAt = combineDateAndTime(targetSlotDate, timeInputFromDate(slot.endsAt))

    setDaysDraft((state) =>
      state.map((day) => {
        if (day.date === slot.slotDate && day.date !== targetSlotDate) {
          return { ...day, slots: day.slots.filter((item) => item.id !== slot.id) }
        }
        if (day.date === slot.slotDate && day.date === targetSlotDate) {
          return { ...day, slots: reorderByIndex(day.slots, slot.id, targetIndex) }
        }
        if (day.date === targetSlotDate) {
          const withoutMoved = day.slots.filter((item) => item.id !== slot.id)
          const nextSlots = [...withoutMoved]
          const insertAt = Math.max(0, Math.min(targetIndex, nextSlots.length))
          nextSlots.splice(insertAt, 0, { ...slot, slotDate: targetSlotDate, startsAt: movedStartsAt, endsAt: movedEndsAt })
          return { ...day, slots: nextSlots }
        }
        return day
      })
    )

    const result = await runAction({
      key: `day-slot-move-${slot.id}`,
      event: 'day_slot_moved',
      execute: () => moveDaySlotMutation.mutateAsync({ id: slot.id, targetSlotDate, targetIndex }),
      successMessage: slot.slotDate === targetSlotDate ? 'Слот переставлен.' : 'Слот перенесен в другой день.',
      invalidateKeys: slotMutationInvalidateKeys(),
      telemetry: { screen: 'slots', entityId: slot.id, targetSlotDate }
    })
    if (!result) {
      setDaysDraft(previousDays)
    }
  }

  const hasDays = useMemo(() => daysDraft.length > 0, [daysDraft])

  return (
    <section className="space-y-6">
      <PageHeader
        title="Слоты"
        description="Неделя идет в одну горизонтальную строку. Нажми плюс у нужного дня, выбери время, впиши текст плашки и выбери цвет. После подтверждения запись сама появится в этом дне с именем клиента."
      />

      <SectionCard title="Неделя" subtitle="Колонки можно листать вправо. Карточки слотов показывают время, текст и клиента, если слот уже подтвержден в записи.">
        {!hasDays ? (
          <EmptyState title="Нет слотов" description="Неделя еще не загружена." />
        ) : (
          <div className="overflow-x-auto pb-2">
            <div className="grid min-w-max grid-flow-col auto-cols-[250px] gap-3">
              {daysDraft.map((day) => {
                const draft = draftsByDate[day.date]
                return (
                  <div
                    key={day.date}
                    data-testid={`day-column-${day.date}`}
                    className="rounded-[16px] border border-[#ebebeb] bg-[#fafafa] p-3"
                    onDragOver={(event) => event.preventDefault()}
                    onDrop={() => {
                      if (!dragSlot) return
                      const draggedDay = daysDraft.find((item) => item.date === dragSlot.slotDate)
                      const slot = draggedDay?.slots.find((item) => item.id === dragSlot.id)
                      if (!slot) return
                      void onMoveDaySlot(slot, day.date, day.slots.length)
                      setDragSlot(null)
                    }}
                  >
                    <div className="flex items-start justify-between gap-2">
                      <div>
                        <h3 className="text-sm font-semibold text-[#292929]">{day.label}</h3>
                        <p className="mt-0.5 text-xs text-[#8e8e8e]">
                          {formatDateLabel(day.date)} · {day.slots.length ? `${day.slots.length} слота` : 'Пусто'}
                        </p>
                      </div>
                      <ActionButton variant="primary" className="h-8 min-w-8 px-2" onClick={() => setOpenCreatorDate((current) => (current === day.date ? null : day.date))}>
                        +
                      </ActionButton>
                    </div>

                    {openCreatorDate === day.date && draft ? (
                      <div className="mt-3 rounded-[12px] border border-[#e7e1ef] bg-white p-3 shadow-sm">
                        <div className="grid grid-cols-2 gap-2">
                          <input
                            type="time"
                            value={draft.startTime}
                            onChange={(event) => setDraftsByDate((state) => ({ ...state, [day.date]: { ...state[day.date], startTime: event.target.value } }))}
                            className="rounded-[10px] border border-[#ebebeb] px-3 py-2 text-sm outline-none focus:border-[#8089a8]"
                          />
                          <input
                            type="time"
                            value={draft.endTime}
                            onChange={(event) => setDraftsByDate((state) => ({ ...state, [day.date]: { ...state[day.date], endTime: event.target.value } }))}
                            className="rounded-[10px] border border-[#ebebeb] px-3 py-2 text-sm outline-none focus:border-[#8089a8]"
                          />
                        </div>
                        <input
                          type="text"
                          value={draft.note}
                          onChange={(event) => setDraftsByDate((state) => ({ ...state, [day.date]: { ...state[day.date], note: event.target.value } }))}
                          className="mt-2 w-full rounded-[10px] border border-[#ebebeb] px-3 py-2 text-sm outline-none focus:border-[#8089a8]"
                        />
                        <div className="mt-2">
                          {renderColorSwatches(
                            colorsDraft,
                            draft.colorPresetId || colorsDraft[0]?.id || '',
                            (id) => {
                              setDraftsByDate((state) => ({ ...state, [day.date]: { ...state[day.date], colorPresetId: id } }))
                              void onCreateDaySlot(day.date, id)
                            },
                            isPending(`day-slot-create-${day.date}`)
                          )}
                        </div>
                        <p className="mt-2 text-[11px] text-[#8e8e8e]">Выбери цвет, и слот сразу появится в колонке дня.</p>
                        <div className="mt-2 flex justify-end">
                          <ActionButton variant="quiet" onClick={() => setOpenCreatorDate(null)}>
                            Закрыть
                          </ActionButton>
                        </div>
                      </div>
                    ) : null}

                    <div className="mt-3 space-y-2">
                      {day.slots.length ? (
                        day.slots.map((slot, index) => {
                          const locked = isLockedSlot(slot)
                          return (
                            <div
                              key={slot.id}
                              data-testid={`slot-card-${slot.id}`}
                              draggable
                              onDragStart={() => setDragSlot({ id: slot.id, slotDate: day.date })}
                              onDragEnd={() => setDragSlot(null)}
                              onDragOver={(event) => event.preventDefault()}
                              onDrop={(event) => {
                                event.preventDefault()
                                event.stopPropagation()
                                if (!dragSlot) return
                                const draggedDay = daysDraft.find((item) => item.date === dragSlot.slotDate)
                                const draggedSlot = draggedDay?.slots.find((item) => item.id === dragSlot.id)
                                const targetIndex = day.slots.findIndex((item) => item.id === slot.id)
                                if (!draggedSlot || targetIndex === -1) return
                                void onMoveDaySlot(draggedSlot, day.date, targetIndex)
                                setDragSlot(null)
                              }}
                              className="rounded-[12px] border p-3"
                              style={{ backgroundColor: slot.colorHex, borderColor: slot.colorHex }}
                            >
                              <div className="grid grid-cols-2 gap-2">
                                <input
                                  type="time"
                                  value={timeInputFromDate(slot.startsAt)}
                                  disabled={locked}
                                  onChange={(event) =>
                                    setDaysDraft((state) =>
                                      state.map((currentDay) =>
                                        currentDay.date !== day.date
                                          ? currentDay
                                          : {
                                              ...currentDay,
                                              slots: currentDay.slots.map((item) =>
                                                item.id !== slot.id
                                                  ? item
                                                  : { ...item, startsAt: combineDateAndTime(item.slotDate, event.target.value) }
                                              )
                                            }
                                      )
                                    )
                                  }
                                  className="rounded-[10px] border border-white/70 bg-white/90 px-2.5 py-2 text-sm outline-none focus:border-[#292929] disabled:cursor-not-allowed disabled:bg-white/70"
                                />
                                <input
                                  type="time"
                                  value={timeInputFromDate(slot.endsAt)}
                                  disabled={locked}
                                  onChange={(event) =>
                                    setDaysDraft((state) =>
                                      state.map((currentDay) =>
                                        currentDay.date !== day.date
                                          ? currentDay
                                          : {
                                              ...currentDay,
                                              slots: currentDay.slots.map((item) => {
                                                if (item.id !== slot.id) return item
                                                const nextDuration = durationFromTimeRange(timeInputFromDate(item.startsAt), event.target.value)
                                                return nextDuration > 0 ? { ...item, durationMinutes: nextDuration, endsAt: combineDateAndTime(item.slotDate, event.target.value) } : item
                                              })
                                            }
                                      )
                                    )
                                  }
                                  className="rounded-[10px] border border-white/70 bg-white/90 px-2.5 py-2 text-sm outline-none focus:border-[#292929] disabled:cursor-not-allowed disabled:bg-white/70"
                                />
                              </div>

                              <div className="mt-2 flex items-center justify-between gap-2">
                                <p className="text-[11px] text-[#292929]/70">{durationLabel(slot.durationMinutes)}</p>
                                {visibleSlotText(slot.customerName || '') ? (
                                  <button
                                    type="button"
                                    data-testid={`slot-customer-${slot.id}`}
                                    onClick={() => void router.navigate({ to: '/dialogs' })}
                                    className="rounded-full bg-white/85 px-2 py-0.5 text-[11px] font-medium text-[#292929] underline-offset-2 hover:underline"
                                  >
                                    {visibleSlotText(slot.customerName)}
                                  </button>
                                ) : null}
                              </div>

                              {locked ? (
                                <button
                                  type="button"
                                  onClick={() => void router.navigate({ to: '/dialogs' })}
                                  className="mt-2 w-full rounded-[10px] border border-white/70 bg-white/90 px-3 py-2 text-left text-sm text-[#292929] outline-none"
                                >
                                  {visibleSlotText(slot.note || '') || '—'}
                                </button>
                              ) : (
                                <input
                                  type="text"
                                  value={slot.note}
                                  onChange={(event) =>
                                    setDaysDraft((state) =>
                                      state.map((currentDay) =>
                                        currentDay.date !== day.date
                                          ? currentDay
                                          : {
                                              ...currentDay,
                                              slots: currentDay.slots.map((item) => (item.id === slot.id ? { ...item, note: event.target.value } : item))
                                            }
                                      )
                                    )
                                  }
                                  className="mt-2 w-full rounded-[10px] border border-white/70 bg-white/90 px-3 py-2 text-sm outline-none focus:border-[#292929]"
                                />
                              )}

                              <div className="mt-2" onClick={(e) => e.stopPropagation()}>
                                {renderColorSwatches(
                                  colorsDraft,
                                  slot.colorPresetId,
                                  (id) => {
                                    if (!locked && id !== slot.colorPresetId) {
                                      void onChangeSlotColor(slot, id)
                                    }
                                  },
                                  locked || isPending(`day-slot-color-${slot.id}`)
                                )}
                              </div>

                              <div className="mt-3 flex flex-wrap gap-1.5">
                                {!locked ? (
                                  <ActionButton type="button" variant="secondary" isLoading={isPending(`day-slot-save-${slot.id}`)} loadingLabel="..." onClick={(e) => { e.stopPropagation(); void onSaveDaySlot(slot) }}>
                                    Сохранить
                                  </ActionButton>
                                ) : null}
                                <ActionButton type="button" variant="danger" isLoading={isPending(`day-slot-delete-${slot.id}`)} loadingLabel="..." onClick={(e) => { e.stopPropagation(); void onDeleteDaySlot(slot) }}>
                                  Удалить
                                </ActionButton>
                              </div>
                            </div>
                          )
                        })
                      ) : (
                        <EmptyState title="Нет слотов" description="Нажми на плюсик и создай первый кирпичик для этого дня." />
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        )}
      </SectionCard>
    </section>
  )
}
