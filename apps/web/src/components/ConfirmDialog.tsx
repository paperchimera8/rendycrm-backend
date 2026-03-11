import type { ReactNode } from 'react'
import { ActionButton } from './ActionButton'

export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel,
  onConfirm,
  onCancel,
  isLoading = false
}: {
  open: boolean
  title: string
  description: string
  confirmLabel: string
  onConfirm: () => void
  onCancel: () => void
  isLoading?: boolean
}) {
  if (!open) return null
  return (
    <div className="fixed inset-0 z-40 grid place-items-center bg-[#252525]/45 p-4 backdrop-blur-sm">
      <div className="w-full max-w-md rounded-[10px] border border-[#ebebeb] bg-white p-6">
        <h3 className="text-xl font-semibold text-[#292929]">{title}</h3>
        <p className="mt-2 text-sm text-[#5e5e5e]">{description}</p>
        <div className="mt-6 flex justify-end gap-2">
          <ActionButton variant="quiet" onClick={onCancel}>
            Отмена
          </ActionButton>
          <ActionButton variant="danger" isLoading={isLoading} loadingLabel="Сохраняем..." onClick={onConfirm}>
            {confirmLabel}
          </ActionButton>
        </div>
      </div>
    </div>
  )
}
