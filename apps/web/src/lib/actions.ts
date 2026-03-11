import { useQueryClient, type QueryKey } from '@tanstack/react-query'
import { useCallback, useRef, useState } from 'react'
import { useUIStore } from '../stores/ui'

type ActionEventName = string

type RunActionOptions<T> = {
  key: string
  event: ActionEventName
  execute: () => Promise<T>
  successMessage?: string
  errorMessage?: string
  invalidateKeys?: QueryKey[]
  telemetry?: Record<string, string | number | boolean | null | undefined>
  onSuccess?: (result: T) => void | Promise<void>
}

export function normalizeActionError(error: unknown): string {
  if (error instanceof Error) {
    const message = error.message.trim()
    if (message.toLowerCase().includes('slot unavailable')) {
      return 'Выбранный слот уже недоступен. Выберите другое время.'
    }
    if (message.toLowerCase().includes('failed to fetch')) {
      return 'Ошибка сети. Попробуйте еще раз.'
    }
    if (message) {
      return message.charAt(0).toUpperCase() + message.slice(1)
    }
  }
  return 'Не удалось выполнить действие. Попробуйте еще раз.'
}

function logTelemetry(event: ActionEventName, payload: Record<string, unknown>) {
  console.info('[telemetry]', event, payload)
}

export function useActionRunner() {
  const queryClient = useQueryClient()
  const pushToast = useUIStore((state) => state.pushToast)
  const [pendingKeys, setPendingKeys] = useState<Record<string, boolean>>({})
  const pendingRef = useRef<Record<string, boolean>>({})

  const runAction = useCallback(
    async <T,>({
      key,
      event,
      execute,
      successMessage,
      errorMessage,
      invalidateKeys = [],
      telemetry = {},
      onSuccess
    }: RunActionOptions<T>): Promise<T | undefined> => {
      if (pendingRef.current[key]) {
        return undefined
      }

      pendingRef.current[key] = true
      setPendingKeys((state) => ({ ...state, [key]: true }))
      logTelemetry(event, { phase: 'clicked', ...telemetry })

      try {
        const result = await execute()
        await Promise.all(
          invalidateKeys.map(async (queryKey) => {
            await queryClient.invalidateQueries({ queryKey })
            await queryClient.refetchQueries({ queryKey, type: 'active' })
          })
        )
        if (successMessage) {
          pushToast('success', successMessage)
        }
        logTelemetry(event, { phase: 'succeeded', ...telemetry })
        await onSuccess?.(result)
        return result
      } catch (error) {
        const message = errorMessage ?? normalizeActionError(error)
        pushToast('error', message)
        logTelemetry(event, { phase: 'failed', message, ...telemetry })
        return undefined
      } finally {
        pendingRef.current[key] = false
        setPendingKeys((state) => ({ ...state, [key]: false }))
      }
    },
    [pushToast, queryClient]
  )

  const isPending = useCallback((key: string) => Boolean(pendingKeys[key]), [pendingKeys])

  return { runAction, isPending }
}
