import { useEffect, useMemo, useState } from "react";
import { ActionButton } from "../components/ActionButton";
import { EmptyState } from "../components/EmptyState";
import { SectionCard } from "../components/SectionCard";
import {
  useBookPublicCalendarMutation,
  usePublicCalendar,
} from "../lib/queries";
import type { DailySlot } from "../lib/types";

function todayLocal() {
  const now = new Date();
  const year = now.getFullYear();
  const month = `${now.getMonth() + 1}`.padStart(2, "0");
  const day = `${now.getDate()}`.padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function addDays(date: string, days: number) {
  const next = new Date(`${date}T00:00:00`);
  next.setDate(next.getDate() + days);
  const year = next.getFullYear();
  const month = `${next.getMonth() + 1}`.padStart(2, "0");
  const day = `${next.getDate()}`.padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function formatDayLabel(date: string, timezone: string) {
  return new Date(`${date}T00:00:00`).toLocaleDateString("ru-RU", {
    weekday: "long",
    day: "numeric",
    month: "long",
    timeZone: timezone,
  });
}

function formatTime(value: string, timezone: string) {
  return new Date(value).toLocaleTimeString("ru-RU", {
    hour: "2-digit",
    minute: "2-digit",
    timeZone: timezone,
  });
}

function formatSlotLabel(slot: DailySlot, timezone: string) {
  return `${formatTime(slot.startsAt, timezone)} - ${formatTime(slot.endsAt, timezone)}`;
}

function formatDateTime(value: string, timezone: string) {
  return new Date(value).toLocaleString("ru-RU", {
    day: "numeric",
    month: "long",
    hour: "2-digit",
    minute: "2-digit",
    timeZone: timezone,
  });
}

export function CalendarPage() {
  const token = useMemo(
    () =>
      new URLSearchParams(window.location.search).get("token")?.trim() ?? "",
    [],
  );
  const dateFrom = useMemo(() => todayLocal(), []);
  const dateTo = useMemo(() => addDays(dateFrom, 13), [dateFrom]);
  const calendar = usePublicCalendar(token, dateFrom, dateTo);
  const bookMutation = useBookPublicCalendarMutation();

  const [selectedSlot, setSelectedSlot] = useState<DailySlot | null>(null);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");

  const timezone = calendar.data?.workspace.timezone || "Europe/Moscow";

  const groupedSlots = useMemo(() => {
    const buckets = new Map<string, DailySlot[]>();
    for (const item of calendar.data?.items ?? []) {
      const dayItems = buckets.get(item.slotDate) ?? [];
      dayItems.push(item);
      buckets.set(item.slotDate, dayItems);
    }
    return Array.from(buckets.entries())
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([date, items]) => ({
        date,
        items: [...items].sort((left, right) =>
          left.startsAt.localeCompare(right.startsAt),
        ),
      }));
  }, [calendar.data?.items]);

  useEffect(() => {
    if (!selectedSlot) return;
    const stillAvailable = (calendar.data?.items ?? []).some(
      (item) => item.id === selectedSlot.id,
    );
    if (!stillAvailable) {
      setSelectedSlot(null);
    }
  }, [calendar.data?.items, selectedSlot]);

  const onBook = async () => {
    if (!selectedSlot || !token) return;
    setError("");
    setSuccess("");
    try {
      const response = await bookMutation.mutateAsync({
        token,
        dailySlotId: selectedSlot.id,
      });
      setSuccess(
        `Запись подтверждена на ${formatDateTime(response.booking.startsAt, timezone)}.`,
      );
      setSelectedSlot(null);
      await calendar.refetch();
    } catch (bookingError) {
      setError(
        bookingError instanceof Error
          ? bookingError.message
          : "Не удалось создать запись.",
      );
      await calendar.refetch();
    }
  };

  return (
    <main className="min-h-screen bg-[#f4f1ea] px-4 py-6 text-[#292929] sm:px-6">
      <div className="mx-auto flex w-full max-w-5xl flex-col gap-4">
        <section className="overflow-hidden rounded-[28px] border border-[#dfd7ca] bg-[linear-gradient(135deg,#fffaf2_0%,#f3eee3_100%)] p-6 shadow-[0_10px_40px_rgba(41,41,41,0.08)]">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
            <div className="space-y-2">
              <p className="text-xs font-semibold uppercase tracking-[0.24em] text-[#8a6f3f]">
                Online Booking
              </p>
              <h1 className="text-3xl font-semibold leading-tight text-[#20170f]">
                {calendar.data?.workspace.name || "Календарь записи"}
              </h1>
              <p className="max-w-2xl text-sm leading-6 text-[#5e5447]">
                Выберите свободный слот. Календарь обновляется автоматически
                каждые 30 секунд и показывает актуальную доступность мастера.
              </p>
            </div>
            <ActionButton
              variant="secondary"
              isLoading={calendar.isFetching}
              loadingLabel="Обновляю..."
              onClick={() => void calendar.refetch()}
            >
              Обновить
            </ActionButton>
          </div>
        </section>

        {!token ? (
          <SectionCard>
            <EmptyState
              title="Ссылка недействительна"
              description="Откройте календарь из кнопки в Telegram, чтобы продолжить запись."
            />
          </SectionCard>
        ) : calendar.isLoading ? (
          <SectionCard
            title="Загрузка календаря"
            subtitle="Получаем актуальные свободные окна."
          >
            <div className="grid gap-3 md:grid-cols-2">
              {Array.from({ length: 4 }).map((_, index) => (
                <div
                  key={index}
                  className="h-28 animate-pulse rounded-[18px] bg-[#f5f5f5]"
                />
              ))}
            </div>
          </SectionCard>
        ) : calendar.isError ? (
          <SectionCard>
            <EmptyState
              title="Не удалось открыть календарь"
              description={
                calendar.error instanceof Error
                  ? calendar.error.message
                  : "Попробуйте открыть ссылку заново из Telegram."
              }
            />
          </SectionCard>
        ) : (
          <>
            <SectionCard
              title="Свободные слоты"
              subtitle={
                groupedSlots.length > 0
                  ? `Период: ${formatDayLabel(dateFrom, timezone)} - ${formatDayLabel(dateTo, timezone)}`
                  : "Свободные окна появятся здесь сразу после синхронизации расписания мастера."
              }
            >
              {groupedSlots.length === 0 ? (
                <EmptyState
                  title="Свободных слотов пока нет"
                  description="Попробуйте обновить страницу чуть позже или напишите мастеру прямо в Telegram."
                />
              ) : (
                <div className="grid gap-3 lg:grid-cols-2">
                  {groupedSlots.map((day) => (
                    <div
                      key={day.date}
                      className="rounded-[22px] border border-[#eee6d9] bg-[#fffdf9] p-4"
                    >
                      <div className="mb-3">
                        <h2 className="text-sm font-semibold capitalize text-[#20170f]">
                          {formatDayLabel(day.date, timezone)}
                        </h2>
                        <p className="text-xs text-[#8b8174]">
                          {day.items.length} слотов доступно
                        </p>
                      </div>
                      <div className="flex flex-wrap gap-2">
                        {day.items.map((slot) => {
                          const active = selectedSlot?.id === slot.id;
                          return (
                            <button
                              key={slot.id}
                              type="button"
                              onClick={() => {
                                setError("");
                                setSuccess("");
                                setSelectedSlot(slot);
                              }}
                              className={`rounded-full border px-3 py-2 text-sm transition ${
                                active
                                  ? "border-[#8a6f3f] bg-[#8a6f3f] text-white shadow-[0_8px_18px_rgba(138,111,63,0.25)]"
                                  : "border-[#e8dcc8] bg-white text-[#3a3127] hover:border-[#c9b28a] hover:bg-[#fff8ed]"
                              }`}
                            >
                              {formatSlotLabel(slot, timezone)}
                            </button>
                          );
                        })}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </SectionCard>

            <SectionCard
              title="Подтверждение"
              subtitle={
                selectedSlot
                  ? "Проверьте выбранный слот и подтвердите запись."
                  : "Выберите слот в календаре выше."
              }
              actions={
                selectedSlot ? (
                  <ActionButton
                    variant="primary"
                    isLoading={bookMutation.isPending}
                    loadingLabel="Подтверждаю..."
                    onClick={() => void onBook()}
                  >
                    Записаться на это время
                  </ActionButton>
                ) : null
              }
            >
              {selectedSlot ? (
                <div className="space-y-2">
                  <p className="text-sm text-[#3a3127]">
                    <span className="font-medium text-[#20170f]">Дата:</span>{" "}
                    {formatDayLabel(selectedSlot.slotDate, timezone)}
                  </p>
                  <p className="text-sm text-[#3a3127]">
                    <span className="font-medium text-[#20170f]">Время:</span>{" "}
                    {formatSlotLabel(selectedSlot, timezone)}
                  </p>
                  {selectedSlot.note ? (
                    <p className="text-sm text-[#3a3127]">
                      <span className="font-medium text-[#20170f]">
                        Комментарий мастера:
                      </span>{" "}
                      {selectedSlot.note}
                    </p>
                  ) : null}
                </div>
              ) : (
                <EmptyState
                  title="Слот не выбран"
                  description="Нажмите на удобное время в списке выше."
                />
              )}
              {success ? (
                <p className="mt-4 rounded-[16px] border border-[#d7ead2] bg-[#f4fbf1] px-4 py-3 text-sm text-[#2b5b33]">
                  {success}
                </p>
              ) : null}
              {error ? (
                <p className="mt-4 rounded-[16px] border border-[#f0d2d2] bg-[#fff5f5] px-4 py-3 text-sm text-[#a33a3a]">
                  {error}
                </p>
              ) : null}
            </SectionCard>
          </>
        )}
      </div>
    </main>
  );
}
