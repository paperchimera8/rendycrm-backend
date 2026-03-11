import { useRouter } from '@tanstack/react-router'
import { ActionButton } from '../components/ActionButton'
import { PageHeader } from '../components/PageHeader'
import { SectionCard } from '../components/SectionCard'
import { StatCard } from '../components/StatCard'
import { useActionRunner } from '../lib/actions'
import { useAnalytics } from '../lib/queries'

function downloadFile(filename: string, content: string, type: string) {
  const blob = new Blob([content], { type })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  URL.revokeObjectURL(url)
}

export function AnalyticsPage() {
  const router = useRouter()
  const analytics = useAnalytics()
  const { runAction, isPending } = useActionRunner()

  const metrics = [
    { label: 'Выручка', value: analytics.data?.revenue ?? 0, unit: 'RUB', cta: 'Открыть записи', to: '/slots' as const },
    { label: 'Подтверждаемость', value: analytics.data?.confirmationRate ?? 0, unit: '%', cta: 'Проверить записи', to: '/slots' as const },
    { label: 'Повторные записи', value: analytics.data?.repeatBookings ?? 0, unit: '%', cta: 'Открыть исходные диалоги', to: '/dialogs' as const },
    { label: 'Диалог -> запись', value: analytics.data?.conversationToBooking ?? 0, unit: '%', cta: 'Открыть диалоги', to: '/dialogs' as const }
  ]

  const onRefresh = async () => {
    await runAction({
      key: 'analytics-refresh',
      event: 'analytics_refresh',
      execute: async () => ({ ok: true }),
      successMessage: 'Аналитика обновлена.',
      invalidateKeys: [['analytics'], ['dashboard']],
      telemetry: { screen: 'analytics' }
    })
  }

  const onExportJson = async () => {
    await runAction({
      key: 'analytics-export-json',
      event: 'analytics_export_json',
      execute: async () => {
        downloadFile('analytics-snapshot.json', JSON.stringify(analytics.data ?? {}, null, 2), 'application/json')
        return { ok: true }
      },
      successMessage: 'Снимок JSON экспортирован.',
      telemetry: { screen: 'analytics' }
    })
  }

  const onExportCsv = async () => {
    await runAction({
      key: 'analytics-export-csv',
      event: 'analytics_export_csv',
      execute: async () => {
        const rows = [
          ['metric', 'value'],
          ...metrics.map((item) => [item.label, String(item.value)])
        ]
        downloadFile('analytics-snapshot.csv', rows.map((row) => row.join(',')).join('\n'), 'text/csv')
        return { ok: true }
      },
      successMessage: 'Снимок CSV экспортирован.',
      telemetry: { screen: 'analytics' }
    })
  }

  return (
    <section className="space-y-6">
      <PageHeader
        title="Аналитика"
        actions={
          <>
            <ActionButton variant="quiet" isLoading={isPending('analytics-refresh') || analytics.isFetching} loadingLabel="..." onClick={() => void onRefresh()}>Обновить</ActionButton>
            <ActionButton variant="quiet" isLoading={isPending('analytics-export-json')} loadingLabel="..." onClick={() => void onExportJson()}>JSON</ActionButton>
            <ActionButton variant="quiet" isLoading={isPending('analytics-export-csv')} loadingLabel="..." onClick={() => void onExportCsv()}>CSV</ActionButton>
          </>
        }
      />

      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {metrics.map((metric) => (
          <StatCard key={metric.label} label={metric.label} value={metric.value} meta={metric.unit}>
            <ActionButton className="mt-2 w-full" variant="secondary" onClick={() => void router.navigate({ to: metric.to })}>
              {metric.cta}
            </ActionButton>
          </StatCard>
        ))}
      </div>
    </section>
  )
}
