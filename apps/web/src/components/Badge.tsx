export function Badge({
  tone,
  label
}: {
  tone: 'warning' | 'success' | 'danger' | 'neutral'
  label: string
}) {
  const classes = {
    neutral: 'bg-[#f7f7f7] text-[#5e5e5e]',
    success: 'bg-emerald-50 text-emerald-700',
    warning: 'bg-amber-50 text-amber-700',
    danger: 'bg-red-50 text-red-700'
  }
  return <span className={`rounded px-2 py-0.5 text-[11px] font-medium ${classes[tone]}`}>{label}</span>
}
