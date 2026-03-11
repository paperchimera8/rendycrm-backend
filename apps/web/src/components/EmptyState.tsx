import type { ReactNode } from 'react'

export function EmptyState({
  title,
  description = '',
  actions
}: {
  title: string
  description?: string
  actions?: ReactNode
}) {
  return (
    <div className="grid min-h-16 place-items-center rounded-md border border-dashed border-[#ebebeb] bg-[#f7f7f7] px-3 py-2 text-center">
      <div>
        <p className="text-sm font-medium text-[#292929]">{title}</p>
        {description ? <p className="mt-0.5 text-xs text-[#8e8e8e]">{description}</p> : null}
        {actions ? <div className="mt-2 flex flex-wrap justify-center gap-2">{actions}</div> : null}
      </div>
    </div>
  )
}
