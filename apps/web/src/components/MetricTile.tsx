import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

const accentBar: Record<string, string> = {
  violet: 'from-[#a78bfa] to-[#8b5cf6]',
  green: 'from-emerald-400 to-emerald-600',
  blue: 'from-blue-400 to-blue-600',
  amber: 'from-amber-400 to-amber-500'
}

export function MetricTile({
  label,
  value,
  hint,
  onClick,
  accent = 'violet',
  className
}: {
  label: string
  value: string | number
  hint?: string
  onClick?: () => void
  accent?: 'violet' | 'green' | 'blue' | 'amber'
  className?: string
}) {
  const Comp = onClick ? 'button' : 'div'
  return (
    <Comp
      type={onClick ? 'button' : undefined}
      onClick={onClick}
      className={cn(
        'relative overflow-hidden rounded-md border border-[#ebebeb] bg-[#f7f7f7] p-3 pl-4 text-left transition-colors',
        onClick && 'cursor-pointer hover:border-[#a78bfa] hover:bg-white hover:shadow-[0_0_0_1px_rgba(139,92,246,0.12)]',
        className
      )}
    >
      <span className={cn('absolute left-0 top-3 bottom-3 w-1 rounded-r bg-gradient-to-b opacity-70', accentBar[accent])} aria-hidden />
      <p className="text-xs font-medium text-[#8e8e8e]">{label}</p>
      <p className="mt-1 text-xl font-semibold tracking-tight text-[#292929]">{value}</p>
      {hint ? <p className="mt-0.5 text-[11px] text-[#8e8e8e]">{hint}</p> : null}
    </Comp>
  )
}
