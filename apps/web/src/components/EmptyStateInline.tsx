import type { ReactNode } from 'react'

export function EmptyStateInline({ text, action }: { text: string; action?: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-2 rounded-md bg-[#f7f7f7] px-3 py-2">
      <p className="text-xs text-[#8e8e8e]">{text}</p>
      {action}
    </div>
  )
}
