import type { ReactNode } from 'react'

export function SectionCard({
  title,
  subtitle,
  actions,
  children,
  className = ''
}: {
  title?: string
  subtitle?: string
  actions?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <section className={`rounded-lg border border-[#ebebeb] bg-white p-4 ${className}`.trim()}>
      {(title || actions) ? (
        <div className="mb-3 flex items-center justify-between gap-3">
          <div>
            {title ? <h3 className="text-sm font-semibold text-[#292929]">{title}</h3> : null}
            {subtitle ? <p className="mt-0.5 text-xs text-[#8e8e8e]">{subtitle}</p> : null}
          </div>
          {actions ? <div className="flex flex-wrap gap-2">{actions}</div> : null}
        </div>
      ) : null}
      {children}
    </section>
  )
}
