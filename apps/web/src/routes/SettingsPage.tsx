import { useEffect, useMemo, useState } from 'react'
import { ActionBar } from '../components/ActionBar'
import { ActionButton } from '../components/ActionButton'
import { EmptyState } from '../components/EmptyState'
import { InlineStatusBadge } from '../components/InlineStatusBadge'
import { PageHeader } from '../components/PageHeader'
import { SectionCard } from '../components/SectionCard'
import { useActionRunner } from '../lib/actions'
import {
  useAvailability,
  useBotConfig,
  useChannels,
  useCreateOperatorBotLinkCodeMutation,
  useMasterProfile,
  useOperatorBotSettings,
  useSimulateWebhookMutation,
  useUnlinkOperatorBotMutation,
  useUpdateAvailabilityRulesMutation,
  useUpdateBotMutation,
  useUpdateMasterProfileMutation,
  useUpdateChannelMutation
} from '../lib/queries'
import type { AvailabilityRule, BotConfig, FAQItem } from '../lib/types'

function minuteToTime(value: number) {
  const hours = `${Math.floor(value / 60)}`.padStart(2, '0')
  const minutes = `${value % 60}`.padStart(2, '0')
  return `${hours}:${minutes}`
}

function timeToMinute(value: string) {
  const [hours, minutes] = value.split(':').map(Number)
  return hours * 60 + minutes
}

export function SettingsPage() {
  const channels = useChannels()
  const masterProfile = useMasterProfile()
  const bot = useBotConfig()
  const operatorBot = useOperatorBotSettings()
  const availability = useAvailability()
  const updateChannel = useUpdateChannelMutation()
  const updateMasterProfile = useUpdateMasterProfileMutation()
  const updateBot = useUpdateBotMutation()
  const createOperatorLink = useCreateOperatorBotLinkCodeMutation()
  const unlinkOperatorBot = useUnlinkOperatorBotMutation()
  const updateAvailabilityRules = useUpdateAvailabilityRulesMutation()
  const simulateWebhook = useSimulateWebhookMutation()
  const { runAction, isPending } = useActionRunner()
  const [configDraft, setConfigDraft] = useState<BotConfig | null>(null)
  const [faqDraft, setFaqDraft] = useState<FAQItem[]>([])
  const [masterPhoneDraft, setMasterPhoneDraft] = useState('')
  const [workStartDraft, setWorkStartDraft] = useState('09:00')
  const [workEndDraft, setWorkEndDraft] = useState('18:00')

  useEffect(() => {
    if (!bot.data) return
    setConfigDraft(bot.data.config)
    setFaqDraft(bot.data.faqItems)
  }, [bot.data])

  useEffect(() => {
    if (!masterProfile.data) return
    setMasterPhoneDraft(masterProfile.data.masterPhoneRaw)
  }, [masterProfile.data])

  useEffect(() => {
    if (!availability.data?.rules?.length) return
    const enabled = availability.data.rules.filter((rule) => rule.enabled)
    if (!enabled.length) return
    const start = Math.min(...enabled.map((rule) => rule.startMinute))
    const end = Math.max(...enabled.map((rule) => rule.endMinute))
    setWorkStartDraft(minuteToTime(start))
    setWorkEndDraft(minuteToTime(end))
  }, [availability.data?.rules])

  const isDirty = useMemo(() => {
    if (!bot.data || !configDraft) return false
    return JSON.stringify(configDraft) !== JSON.stringify(bot.data.config) || JSON.stringify(faqDraft) !== JSON.stringify(bot.data.faqItems)
  }, [bot.data, configDraft, faqDraft])

  const onToggleChannel = async (provider: string, connected: boolean, name: string) => {
    await runAction({
      key: `channel-${provider}`,
      event: connected ? 'channel_disconnected' : 'channel_connected',
      execute: () => updateChannel.mutateAsync({ provider, connected: !connected, name }),
      successMessage: connected ? `${name} отключен.` : `${name} подключен.`,
      invalidateKeys: [['channels']],
      telemetry: { screen: 'settings', provider }
    })
  }

  const onCopyWebhook = async (provider: string, webhookUrl: string) => {
    await runAction({
      key: `copy-webhook-${provider}`,
      event: 'webhook_copied',
      execute: async () => {
        await navigator.clipboard.writeText(webhookUrl)
        return { ok: true }
      },
      successMessage: 'Webhook URL скопирован.',
      telemetry: { screen: 'settings', provider }
    })
  }

  const onTestWebhook = async (provider: 'telegram' | 'whatsapp') => {
    await runAction({
      key: `test-webhook-${provider}`,
      event: 'webhook_tested',
      execute: () => simulateWebhook.mutateAsync({ provider, customerName: 'Тестовый лид из настроек', text: `Проверка канала ${provider}` }),
      successMessage: `Тестовый лид ${provider} отправлен во входящие.`,
      invalidateKeys: [['conversations'], ['dashboard']],
      telemetry: { screen: 'settings', provider }
    })
  }

  const onSaveBot = async () => {
    if (!configDraft) return
    await runAction({
      key: 'bot-save',
      event: 'bot_config_saved',
      execute: () => updateBot.mutateAsync({ config: configDraft, faqItems: faqDraft }),
      successMessage: 'Настройки бота сохранены.',
      invalidateKeys: [['bot-config']],
      telemetry: { screen: 'settings' }
    })
  }

  const onSaveMasterPhone = async () => {
    await runAction({
      key: 'master-phone-save',
      event: 'master_phone_saved',
      execute: () => updateMasterProfile.mutateAsync({ masterPhone: masterPhoneDraft }),
      successMessage: 'Номер мастера сохранен.',
      invalidateKeys: [['master-profile'], ['channels']],
      telemetry: { screen: 'settings', entity: 'master-profile' }
    })
  }

  const onCreateOperatorLink = async () => {
    await runAction({
      key: 'operator-bot-link',
      event: 'operator_bot_link_created',
      execute: () => createOperatorLink.mutateAsync(),
      successMessage: 'Ссылка для привязки Telegram operator bot создана.',
      invalidateKeys: [['operator-bot']],
      telemetry: { screen: 'settings', entity: 'operator-bot' }
    })
  }

  const onUnlinkOperatorBot = async () => {
    await runAction({
      key: 'operator-bot-unlink',
      event: 'operator_bot_unlinked',
      execute: () => unlinkOperatorBot.mutateAsync(),
      successMessage: 'Telegram operator bot отвязан.',
      invalidateKeys: [['operator-bot']],
      telemetry: { screen: 'settings', entity: 'operator-bot' }
    })
  }

  const onDiscardBot = () => {
    if (!bot.data) return
    setConfigDraft(bot.data.config)
    setFaqDraft(bot.data.faqItems)
  }

  const onSaveWorkHours = async () => {
    await runAction({
      key: 'work-hours-save',
      event: 'work_hours_saved',
      execute: () => {
        const startMinute = timeToMinute(workStartDraft)
        const endMinute = timeToMinute(workEndDraft)
        if (endMinute <= startMinute) {
          throw new Error('Время окончания должно быть позже времени начала.')
        }
        const existingByDay = new Map<number, AvailabilityRule>(
          (availability.data?.rules ?? []).map((rule) => [rule.dayOfWeek, rule])
        )
        const nextRules: AvailabilityRule[] = [0, 1, 2, 3, 4, 5, 6].map((dayOfWeek) => {
          const current = existingByDay.get(dayOfWeek)
          return {
            id: current?.id ?? '',
            dayOfWeek,
            startMinute,
            endMinute,
            enabled: true
          }
        })
        return updateAvailabilityRules.mutateAsync(nextRules)
      },
      successMessage: 'Рабочее время сохранено.',
      invalidateKeys: [['availability'], ['available-slots'], ['week-slots'], ['slot-editor']],
      telemetry: { screen: 'settings' }
    })
  }

  const addFaq = () => {
    setFaqDraft((state) => [...state, { id: `tmp_faq_${Date.now()}`, question: '', answer: '' }])
  }

  const updateFaq = (id: string, patch: Partial<FAQItem>) => {
    setFaqDraft((state) => state.map((item) => (item.id === id ? { ...item, ...patch } : item)))
  }

  const removeFaq = (id: string) => {
    setFaqDraft((state) => state.filter((item) => item.id !== id))
  }

  return (
    <section className="space-y-6">
      <PageHeader
        title="Настройки"
        description={isDirty ? 'Есть несохраненные изменения' : ''}
        actions={
          <>
            <ActionButton variant="quiet" isDisabled={!isDirty} onClick={onDiscardBot}>Отмена</ActionButton>
            <ActionButton variant="primary" isLoading={isPending('bot-save')} isDisabled={!isDirty || !configDraft} loadingLabel="..." onClick={() => void onSaveBot()}>
              Сохранить
            </ActionButton>
          </>
        }
      />

      <div className="grid gap-4 xl:grid-cols-[1fr_1fr]">
        <SectionCard title="Номер мастера">
          <div className="space-y-3">
            <label className="text-sm text-[#5e5e5e]">
              <span className="mb-1 block text-xs font-medium text-[#8e8e8e]">Телефон для общего client bot</span>
              <input
                type="tel"
                value={masterPhoneDraft}
                onChange={(event) => setMasterPhoneDraft(event.target.value)}
                placeholder="+7 999 111-22-33"
                className="w-full rounded-[10px] border border-[#ebebeb] bg-white px-4 py-3 text-[#292929] outline-none focus:border-[#8089a8]"
              />
            </label>
            <div className="flex flex-wrap items-center gap-3">
              <ActionButton variant="primary" isLoading={isPending('master-phone-save')} loadingLabel="..." onClick={() => void onSaveMasterPhone()}>
                Сохранить
              </ActionButton>
              {masterProfile.data?.masterPhoneNormalized ? (
                <InlineStatusBadge tone="success" label={`Нормализован: ${masterProfile.data.masterPhoneNormalized}`} />
              ) : (
                <InlineStatusBadge tone="neutral" label="Номер не задан" />
              )}
            </div>
            <p className="text-xs text-[#8e8e8e]">
              Этот номер клиент вводит в общем Telegram client bot, чтобы попасть в ваш кабинет.
            </p>
          </div>
        </SectionCard>

        <SectionCard title="Рабочее время мастера">
          <div className="grid gap-3 sm:grid-cols-[1fr_1fr_auto] sm:items-end">
            <label className="text-sm text-[#5e5e5e]">
              <span className="mb-1 block text-xs font-medium text-[#8e8e8e]">Начало</span>
              <input
                type="time"
                value={workStartDraft}
                onChange={(event) => setWorkStartDraft(event.target.value)}
                className="w-full rounded-[10px] border border-[#ebebeb] bg-white px-4 py-3 text-[#292929] outline-none focus:border-[#8089a8]"
              />
            </label>
            <label className="text-sm text-[#5e5e5e]">
              <span className="mb-1 block text-xs font-medium text-[#8e8e8e]">Конец</span>
              <input
                type="time"
                value={workEndDraft}
                onChange={(event) => setWorkEndDraft(event.target.value)}
                className="w-full rounded-[10px] border border-[#ebebeb] bg-white px-4 py-3 text-[#292929] outline-none focus:border-[#8089a8]"
              />
            </label>
            <ActionButton variant="primary" isLoading={isPending('work-hours-save')} loadingLabel="..." onClick={() => void onSaveWorkHours()}>
              Сохранить
            </ActionButton>
          </div>
          <p className="mt-3 text-xs text-[#8e8e8e]">
            Свободные окна рассчитываются с шагом 1 час в рамках этого интервала.
          </p>
        </SectionCard>

        <SectionCard title="Каналы">
          {channels.data?.items.length ? (
            <div className="space-y-3">
              {channels.data.items.map((channel) => (
                <div key={channel.id} className="rounded-[10px] border border-[#ebebeb] bg-[#f7f7f7] p-4">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <div className="flex items-center gap-2">
                        <p className="font-semibold text-[#292929]">{channel.name}</p>
                        <InlineStatusBadge tone={channel.connected ? 'success' : 'neutral'} label={channel.connected ? 'подключен' : 'отключен'} />
                      </div>
                      <p className="mt-1 text-sm uppercase tracking-[0.2em] text-[#8e8e8e]">{channel.provider}</p>
                      <p className="mt-3 break-all text-sm text-[#5e5e5e]">Webhook: {channel.webhookUrl}</p>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <ActionButton variant={channel.connected ? 'danger' : 'secondary'} isLoading={isPending(`channel-${channel.provider}`)} loadingLabel="..." onClick={() => void onToggleChannel(channel.provider, channel.connected, channel.name)}>
                        {channel.connected ? 'Откл.' : 'Подкл.'}
                      </ActionButton>
                      <ActionButton variant="quiet" isLoading={isPending(`copy-webhook-${channel.provider}`)} loadingLabel="..." onClick={() => void onCopyWebhook(channel.provider, channel.webhookUrl)}>Копировать</ActionButton>
                      <ActionButton variant="quiet" isLoading={isPending(`test-webhook-${channel.provider}`)} loadingLabel="..." onClick={() => void onTestWebhook(channel.provider)}>Тест</ActionButton>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <EmptyState title="Каналы не настроены" />
          )}
        </SectionCard>

        <SectionCard title="Telegram operator bot">
          {operatorBot.data ? (
            <div className="space-y-3">
              <div className="rounded-[10px] border border-[#ebebeb] bg-[#f7f7f7] p-4">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <div className="flex items-center gap-2">
                      <p className="font-semibold text-[#292929]">@{operatorBot.data.botUsername.replace(/^@/, '')}</p>
                      <InlineStatusBadge
                        tone={operatorBot.data.binding ? 'success' : 'neutral'}
                        label={operatorBot.data.binding ? 'привязан' : 'не привязан'}
                      />
                    </div>
                    <p className="mt-2 text-sm text-[#5e5e5e]">Webhook: {operatorBot.data.operatorWebhookUrl}</p>
                    {operatorBot.data.binding ? (
                      <p className="mt-2 text-sm text-[#5e5e5e]">
                        Telegram chat: {operatorBot.data.binding.telegramChatId}
                      </p>
                    ) : null}
                    {operatorBot.data.pendingLink ? (
                      <div className="mt-3 rounded-[10px] border border-dashed border-[#d8d8d8] bg-white p-3 text-sm text-[#5e5e5e]">
                        <p>Активная ссылка: {operatorBot.data.pendingLink.deepLink}</p>
                        <p className="mt-1">Код: {operatorBot.data.pendingLink.code}</p>
                      </div>
                    ) : null}
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <ActionButton variant="secondary" isLoading={isPending('operator-bot-link')} loadingLabel="..." onClick={() => void onCreateOperatorLink()}>
                      Создать ссылку
                    </ActionButton>
                    {operatorBot.data.pendingLink ? (
                      <ActionButton variant="quiet" onClick={() => void onCopyWebhook('operator-bot', operatorBot.data.pendingLink!.deepLink)}>Копировать link</ActionButton>
                    ) : null}
                    {operatorBot.data.binding ? (
                      <ActionButton variant="danger" isLoading={isPending('operator-bot-unlink')} loadingLabel="..." onClick={() => void onUnlinkOperatorBot()}>
                        Отвязать
                      </ActionButton>
                    ) : null}
                  </div>
                </div>
              </div>
            </div>
          ) : (
            <EmptyState title="Статус operator bot недоступен" />
          )}
        </SectionCard>

        <SectionCard title="Бот и FAQ">
          {configDraft ? (
            <div className="space-y-3">
              <div className="flex flex-wrap gap-2">
                <ActionButton variant={configDraft.autoReply ? 'secondary' : 'quiet'} onClick={() => setConfigDraft({ ...configDraft, autoReply: !configDraft.autoReply })}>
                  {configDraft.autoReply ? 'Автоответ вкл' : 'Автоответ выкл'}
                </ActionButton>
                <ActionButton variant={configDraft.handoffEnabled ? 'secondary' : 'quiet'} onClick={() => setConfigDraft({ ...configDraft, handoffEnabled: !configDraft.handoffEnabled })}>
                  {configDraft.handoffEnabled ? 'Handoff вкл' : 'Handoff выкл'}
                </ActionButton>
              </div>

              <label className="block text-sm">
                <span className="mb-1 block text-xs font-medium text-[#8e8e8e]">Тон</span>
                <input
                  value={configDraft.tone}
                  onChange={(event) => setConfigDraft({ ...configDraft, tone: event.target.value })}
                  className="w-full rounded-[10px] border border-[#ebebeb] bg-white px-4 py-3 text-[#292929] outline-none focus:border-[#8089a8]"
                />
              </label>

              <div className="rounded-lg border border-[#ebebeb] bg-[#f7f7f7] p-3">
                <div className="flex items-center justify-between">
                  <span className="text-sm font-medium text-[#292929]">FAQ</span>
                  <ActionButton variant="secondary" onClick={addFaq}>+ Добавить</ActionButton>
                </div>
                <div className="mt-4 space-y-3">
                  {faqDraft.length ? (
                    faqDraft.map((item) => (
                      <div key={item.id} className="rounded-[10px] border border-[#ebebeb] bg-[#f7f7f7] p-4">
                        <div className="grid gap-3">
                          <label className="text-sm text-[#5e5e5e]">
                            <span className="mb-2 block text-xs uppercase tracking-[0.2em] text-[#8e8e8e]">Вопрос</span>
                            <input
                              value={item.question}
                              onChange={(event) => updateFaq(item.id, { question: event.target.value })}
                              className="w-full rounded-[10px] border border-[#ebebeb] bg-white px-4 py-3 text-[#292929] outline-none focus:border-[#8089a8]"
                            />
                          </label>
                          <label className="text-sm text-[#5e5e5e]">
                            <span className="mb-2 block text-xs uppercase tracking-[0.2em] text-[#8e8e8e]">Ответ</span>
                            <textarea
                              value={item.answer}
                              onChange={(event) => updateFaq(item.id, { answer: event.target.value })}
                              className="min-h-24 w-full rounded-[10px] border border-[#ebebeb] bg-white px-4 py-3 text-[#292929] outline-none focus:border-[#8089a8]"
                            />
                          </label>
                          <ActionButton variant="danger" onClick={() => removeFaq(item.id)}>Удалить</ActionButton>
                        </div>
                      </div>
                    ))
                  ) : (
                    <EmptyState title="FAQ пуст" actions={<ActionButton variant="quiet" onClick={addFaq}>+ Добавить</ActionButton>} />
                  )}
                </div>
              </div>
            </div>
          ) : (
            <EmptyState title="Конфиг недоступен" />
          )}
        </SectionCard>
      </div>
    </section>
  )
}
