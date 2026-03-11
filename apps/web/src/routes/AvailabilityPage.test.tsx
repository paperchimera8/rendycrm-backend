import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  navigate: vi.fn(),
  runAction: vi.fn(),
  useSlotEditor: vi.fn(),
  useWeekSlots: vi.fn(),
  mutateAsync: vi.fn()
}))

vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual<typeof import('@tanstack/react-router')>('@tanstack/react-router')
  return {
    ...actual,
    useRouter: () => ({ navigate: mocks.navigate })
  }
})

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
  useSlotEditor: mocks.useSlotEditor,
  useWeekSlots: mocks.useWeekSlots,
  useCreateDaySlotMutation: () => ({ mutateAsync: mocks.mutateAsync }),
  useUpdateDaySlotMutation: () => ({ mutateAsync: mocks.mutateAsync }),
  useMoveDaySlotMutation: () => ({ mutateAsync: mocks.mutateAsync }),
  useDeleteDaySlotMutation: () => ({ mutateAsync: mocks.mutateAsync })
}))

import { AvailabilityPage } from './AvailabilityPage'

describe('AvailabilityPage', () => {
  it('renders booked slots with the customer name and locked state', () => {
    mocks.useSlotEditor.mockReturnValue({
      data: {
        settings: {
          workspaceId: 'ws_demo',
          timezone: 'Europe/Moscow',
          defaultDurationMinutes: 60,
          generationHorizonDays: 30
        },
        colors: [{ id: 'clr_1', workspaceId: 'ws_demo', name: 'Базовый', hex: '#C7D2FE', position: 0 }],
        weekTemplates: [],
        daySlots: []
      }
    })
    mocks.useWeekSlots.mockReturnValue({
      data: {
        days: [
          {
            date: '2026-03-11',
            weekday: 3,
            label: 'Среда',
            slots: [
              {
                id: 'dsl_booked',
                workspaceId: 'ws_demo',
                slotDate: '2026-03-11',
                startsAt: '2026-03-11T09:00:00.000Z',
                endsAt: '2026-03-11T10:00:00.000Z',
                durationMinutes: 60,
                colorPresetId: 'clr_1',
                colorName: 'Базовый',
                colorHex: '#C7D2FE',
                position: 0,
                status: 'booked',
                sourceTemplateId: '',
                isManual: true,
                note: 'Маникюр',
                bookingId: 'bok_1',
                customerName: 'Integration Customer'
              }
            ]
          }
        ]
      }
    })

    render(<AvailabilityPage />)

    expect(screen.getByText('Integration Customer')).toBeInTheDocument()
    expect(screen.getByText('Маникюр')).toBeInTheDocument()
    expect(screen.queryByText('Сохранить')).not.toBeInTheDocument()
  })
})
