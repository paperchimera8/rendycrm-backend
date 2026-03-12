import { useMemo } from 'react'
import { useRouter } from '@tanstack/react-router'
import { ActionButton } from '../components/ActionButton'
import { EmptyState } from '../components/EmptyState'
import { InlineStatusBadge } from '../components/InlineStatusBadge'
import { PageHeader } from '../components/PageHeader'
import { SectionCard } from '../components/SectionCard'
import { useActionRunner } from '../lib/actions'
import { useBookings, useConversations, useReviewStatusMutation, useReviews } from '../lib/queries'
import { useUIStore } from '../stores/ui'

function reviewStatusLabel(status: string) {
  if (status === 'open') return 'открыт'
  if (status === 'resolved') return 'решен'
  return status
}

export function ReviewsPage() {
  const router = useRouter()
  const reviews = useReviews()
  const conversations = useConversations()
  const bookings = useBookings()
  const filter = useUIStore((state) => state.reviewFilter)
  const setFilter = useUIStore((state) => state.setReviewFilter)
  const setSelectedConversationId = useUIStore((state) => state.setSelectedConversationId)
  const mutation = useReviewStatusMutation()
  const { runAction, isPending } = useActionRunner()

  const items = useMemo(() => {
    const list = reviews.data?.items ?? []
    if (filter === 'all') return list
    return list.filter((review) => review.status === filter)
  }, [reviews.data?.items, filter])

  const openReviews = useMemo(() => items.filter((review) => review.status === 'open'), [items])

  const bookingsById = useMemo(() => new Map((bookings.data?.items ?? []).map((booking) => [booking.id, booking])), [bookings.data?.items])

  const conversationByCustomerId = useMemo(
    () => new Map((conversations.data?.items ?? []).map((conversation) => [conversation.customerId, conversation])),
    [conversations.data?.items]
  )

  const onToggleStatus = async (id: string, status: 'open' | 'resolved') => {
    await runAction({
      key: `review-${id}`,
      event: status === 'resolved' ? 'review_resolved' : 'review_reopened',
      execute: () => mutation.mutateAsync({ id, status }),
      successMessage: status === 'resolved' ? 'Отзыв отмечен как обработанный.' : 'Отзыв снова открыт.',
      invalidateKeys: [['reviews'], ['dashboard']],
      telemetry: { screen: 'reviews', entityId: id }
    })
  }

  const onResolveAllVisible = async () => {
    if (!openReviews.length) return
    await runAction({
      key: 'reviews-resolve-all',
      event: 'reviews_resolve_all',
      execute: async () => {
        await Promise.all(openReviews.map((review) => mutation.mutateAsync({ id: review.id, status: 'resolved' })))
        return { ok: true }
      },
      successMessage: 'Все видимые отзывы обработаны.',
      invalidateKeys: [['reviews'], ['dashboard']],
      telemetry: { screen: 'reviews', count: openReviews.length }
    })
  }

  const onRefresh = async () => {
    await runAction({
      key: 'reviews-refresh',
      event: 'reviews_refresh',
      execute: async () => ({ ok: true }),
      successMessage: 'Очередь отзывов обновлена.',
      invalidateKeys: [['reviews'], ['dashboard']],
      telemetry: { screen: 'reviews' }
    })
  }

  const openCustomer = async (customerId: string) => {
    const match = conversations.data?.items.find((conversation) => conversation.customerId === customerId)
    if (match) {
      setSelectedConversationId(match.id)
      await router.navigate({ to: '/dialogs' })
      return
    }
    await router.navigate({ to: '/dialogs' })
  }

  const openRelatedBooking = async () => {
    await router.navigate({ to: '/slots' })
  }

  const formatReviewDate = (value: string) =>
    new Date(value).toLocaleDateString('ru-RU', {
      day: 'numeric',
      month: 'long',
      hour: '2-digit',
      minute: '2-digit'
    })

  const formatBookingDate = (value: string) =>
    new Date(value).toLocaleDateString('ru-RU', {
      day: '2-digit',
      month: '2-digit',
      hour: '2-digit',
      minute: '2-digit'
    })

  return (
    <section className="space-y-6">
      <PageHeader
        title="Отзывы"
        actions={
          <>
            {(['all', 'open', 'resolved'] as const).map((item) => (
              <ActionButton key={item} variant={filter === item ? 'primary' : 'quiet'} onClick={() => setFilter(item)}>
                {item === 'all' ? 'Все' : item === 'open' ? 'Открытые' : 'Решенные'}
              </ActionButton>
            ))}
            <ActionButton variant="secondary" isLoading={isPending('reviews-resolve-all')} isDisabled={!openReviews.length} loadingLabel="..." onClick={() => void onResolveAllVisible()}>
              Решить все ({openReviews.length})
            </ActionButton>
          </>
        }
      />

      <SectionCard title="Очередь">
        {items.length ? (
          <div className="grid gap-4 xl:grid-cols-2">
            {items.map((review) => {
              const booking = review.bookingId ? bookingsById.get(review.bookingId) : undefined
              const conversation = conversationByCustomerId.get(review.customerId)
              const customerName = booking?.customerName || conversation?.title || 'Клиент'
              const sourceLabel = conversation?.provider === 'telegram' ? 'Telegram' : conversation?.provider === 'whatsapp' ? 'WhatsApp' : null
              const ratingValue = Number.isFinite(review.rating) ? review.rating : 0

              return (
                <article key={review.id} className="rounded-[10px] border border-[#ebebeb] bg-[#f7f7f7] p-4">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-semibold text-[#292929]">{customerName}</p>
                      <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-[#767676]">
                        <span className="font-medium text-[#575757]">{ratingValue.toFixed(1)} ★</span>
                        <InlineStatusBadge tone={review.status === 'open' ? 'danger' : 'success'} label={reviewStatusLabel(review.status)} />
                        <span>{formatReviewDate(review.createdAt)}</span>
                        {booking ? <span>после записи {formatBookingDate(booking.startsAt)}</span> : null}
                        {sourceLabel ? <span>{sourceLabel}</span> : null}
                      </div>
                    </div>
                  </div>

                  <p className="mt-3 text-sm leading-6 text-[#292929]">{review.text}</p>

                  <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-[#7a7a7a]">
                    <button type="button" className="underline-offset-2 hover:text-[#292929] hover:underline" onClick={() => void openCustomer(review.customerId)}>
                      Клиент: {customerName}
                    </button>
                    {booking ? (
                      <button type="button" className="underline-offset-2 hover:text-[#292929] hover:underline" onClick={() => void openRelatedBooking()}>
                        Запись: {formatBookingDate(booking.startsAt)}
                      </button>
                    ) : null}
                  </div>

                  <div className="mt-3 flex flex-wrap gap-2">
                    <ActionButton
                      variant={review.status === 'open' ? 'primary' : 'quiet'}
                      isLoading={isPending(`review-${review.id}`)}
                      loadingLabel="..."
                      onClick={() => void onToggleStatus(review.id, review.status === 'open' ? 'resolved' : 'open')}
                    >
                      {review.status === 'open' ? 'Отметить решенным' : 'Вернуть в работу'}
                    </ActionButton>
                    <ActionButton variant="secondary" onClick={() => void openCustomer(review.customerId)}>
                      Открыть диалог
                    </ActionButton>
                  </div>
                </article>
              )
            })}
          </div>
        ) : (
          <EmptyState title={filter === 'open' ? 'Нет открытых' : 'Нет отзывов'} actions={filter === 'open' ? <ActionButton variant="quiet" onClick={() => setFilter('all')}>Все</ActionButton> : undefined} />
        )}
      </SectionCard>
    </section>
  )
}
