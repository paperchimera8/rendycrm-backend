import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

export function Section({
  title,
  action,
  children,
  className
}: {
  title: string
  action?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <section className={cn(className)}>
      <div className="mb-2 flex items-center justify-between">
        <h3 className="text-sm font-bold text-[#292929]"><span className="border-b border-[#8b5cf6]/40 pb-0.5">{title}</span></h3>
        {action}
      </div>
      {children}
    </section>
  )
}
