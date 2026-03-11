import type { ReactNode } from 'react'
import { Card } from '@/components/ui/card'
import { ArrowRight } from 'lucide-react'
import { cn } from '@/lib/utils'

export function ListItemCard({
  title,
  subtitle,
  meta,
  badge,
  actionLabel = 'Open',
  onClick,
  className
}: {
  title: string
  subtitle?: string
  meta?: string
  badge?: ReactNode
  actionLabel?: string
  onClick?: () => void
  className?: string
}) {
  const Comp = onClick ? 'button' : 'div'
  return (
    <Comp type={onClick ? 'button' : undefined} onClick={onClick} className="w-full text-left">
      <Card
        className={cn(
          'group relative overflow-hidden border border-border bg-card py-0 gap-0 hover:bg-accent/50 transition-colors',
          onClick && 'cursor-pointer',
          className
        )}
      >
        <div className="flex items-center justify-between gap-3 px-3 py-2">
          <div className="min-w-0 flex-1 space-y-0.5">
            <div className="flex items-center gap-2">
              <h3 className="text-sm font-medium text-foreground truncate">{title}</h3>
              {badge && <div className="shrink-0">{badge}</div>}
            </div>
            {subtitle && <p className="text-xs text-muted-foreground line-clamp-1">{subtitle}</p>}
            {meta && <p className="text-[11px] text-muted-foreground/70">{meta}</p>}
          </div>
          {onClick && (
            <span className="shrink-0 flex items-center gap-1 text-[11px] font-medium text-[#7c3aed] group-hover:text-[#6d28d9] transition-colors">
              {actionLabel}
              <ArrowRight className="size-3 group-hover:translate-x-0.5 transition-transform" />
            </span>
          )}
        </div>
      </Card>
    </Comp>
  )
}
