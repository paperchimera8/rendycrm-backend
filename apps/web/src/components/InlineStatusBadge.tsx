export function InlineStatusBadge({
  tone,
  label
}: {
  tone: 'neutral' | 'success' | 'warning' | 'danger'
  label: string
}) {
  const classes = {
    neutral: 'bg-[#f7f7f7] text-[#5e5e5e]',
    success: 'bg-emerald-50 text-emerald-700',
    warning: 'bg-amber-50 text-amber-700',
    danger: 'bg-red-50 text-red-700'
  }

  return <span className={`rounded-full px-3 py-1 text-[11px] uppercase tracking-[0.2em] ${classes[tone]}`}>{label}</span>
}
