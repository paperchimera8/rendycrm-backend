import type { ReactNode } from 'react'

export function PageHeader({
  title,
  description,
  actions
}: {
  title: string
  description?: string
  actions?: ReactNode
}) {
  return (
    <header className="mb-4 flex flex-col gap-3 border-b border-[#ebebeb] pb-4 lg:flex-row lg:items-center lg:justify-between">
      <div>
        <h2 className="text-xl font-semibold tracking-tight text-[#292929]">{title}</h2>
        {description ? <p className="mt-0.5 text-sm text-[#8e8e8e]">{description}</p> : null}
      </div>
      {actions ? <div className="flex flex-wrap gap-2">{actions}</div> : null}
    </header>
  )
}
