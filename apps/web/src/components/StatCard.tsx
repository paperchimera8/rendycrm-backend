import type { ReactNode } from 'react'

export function StatCard({
  label,
  value,
  meta,
  children
}: {
  label: string
  value: ReactNode
  meta?: string
  children?: ReactNode
}) {
  return (
    <div className="rounded-[10px] border border-[#ebebeb] bg-white p-5">
      <p className="text-xs font-medium text-[#8e8e8e]">{label}</p>
      <p className="mt-2 text-2xl font-semibold tracking-tight text-[#292929]">{value}</p>
      {meta ? <p className="mt-2 text-sm text-[#5e5e5e]">{meta}</p> : null}
      {children ? <div className="mt-4">{children}</div> : null}
    </div>
  )
}
