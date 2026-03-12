import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  runAction: vi.fn(),
  useChannels: vi.fn(),
  useMasterProfile: vi.fn(),
  useBotConfig: vi.fn(),
  useOperatorBotSettings: vi.fn(),
  useAvailability: vi.fn(),
  mutateAsync: vi.fn()
}))

vi.mock('../lib/actions', async () => {
  const actual = await vi.importActual<typeof import('../lib/actions')>('../lib/actions')
  return {
    ...actual,
    useActionRunner: () => ({
      runAction: mocks.runAction,
      isPending: () => false
    })
  }
})

vi.mock('../lib/queries', () => ({
  useChannels: mocks.useChannels,
  useMasterProfile: mocks.useMasterProfile,
  useBotConfig: mocks.useBotConfig,
  useOperatorBotSettings: mocks.useOperatorBotSettings,
  useAvailability: mocks.useAvailability,
  useUpdateChannelMutation: () => ({ mutateAsync: mocks.mutateAsync }),
  useUpdateChannelBotMutation: () => ({ mutateAsync: mocks.mutateAsync }),
  useUpdateMasterProfileMutation: () => ({ mutateAsync: mocks.mutateAsync }),
  useUpdateBotMutation: () => ({ mutateAsync: mocks.mutateAsync }),
  useUpdateOperatorBotMutation: () => ({ mutateAsync: mocks.mutateAsync }),
  useCreateOperatorBotLinkCodeMutation: () => ({ mutateAsync: mocks.mutateAsync }),
  useUnlinkOperatorBotMutation: () => ({ mutateAsync: mocks.mutateAsync }),
  useUpdateAvailabilityRulesMutation: () => ({ mutateAsync: mocks.mutateAsync }),
  useSimulateWebhookMutation: () => ({ mutateAsync: mocks.mutateAsync })
}))

import { SettingsPage } from './SettingsPage'

describe('SettingsPage', () => {
  it('renders operator bot binding and pending deep link', () => {
    mocks.useChannels.mockReturnValue({
      data: {
        items: []
      }
    })
    mocks.useBotConfig.mockReturnValue({
      data: {
        config: {
          workspaceId: 'ws_demo',
          autoReply: true,
          handoffEnabled: true,
          faqCount: 1,
          tone: 'helpful'
        },
        faqItems: [{ id: 'faq_1', question: 'Q', answer: 'A' }]
      }
    })
    mocks.useOperatorBotSettings.mockReturnValue({
      data: {
        binding: {
          userId: 'usr_1',
          workspaceId: 'ws_demo',
          telegramUserId: 'tg-user-1',
          telegramChatId: 'tg-chat-1',
          linkedAt: '2026-03-10T12:00:00.000Z',
          isActive: true
        },
        pendingLink: {
          id: 'obl_1',
          userId: 'usr_1',
          workspaceId: 'ws_demo',
          code: 'link_123',
          expiresAt: '2026-03-10T12:30:00.000Z',
          deepLink: 'https://t.me/rendycrm_operator_bot?start=link_123'
        },
        botUsername: 'rendycrm_operator_bot',
        operatorWebhookUrl: 'http://127.0.0.1:8080/webhooks/telegram/operator',
        tokenConfigured: true
      }
    })
    mocks.useAvailability.mockReturnValue({
      data: {
        rules: [{ id: 'avr_1', dayOfWeek: 1, startMinute: 9 * 60, endMinute: 18 * 60, enabled: true }]
      }
    })
    mocks.useMasterProfile.mockReturnValue({
      data: {
        workspaceId: 'ws_demo',
        masterPhoneRaw: '+7 (999) 111-22-33',
        masterPhoneNormalized: '79991112233',
        telegramEnabled: true
      }
    })

    render(<SettingsPage />)

    expect(screen.getByDisplayValue('rendycrm_operator_bot')).toBeInTheDocument()
    expect(screen.getByText(/Привязан: tg-chat-1/)).toBeInTheDocument()
    expect(screen.getByText(/https:\/\/t\.me\/rendycrm_operator_bot\?start=link_123/)).toBeInTheDocument()
    expect(screen.getByPlaceholderText('+7')).toBeInTheDocument()
  })
})
