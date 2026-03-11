import type { ReactNode } from 'react'

export function ActionBar({ children }: { children: ReactNode }) {
  return <div className="flex flex-wrap items-center gap-2 rounded-[10px] border border-[#ebebeb] bg-[#f7f7f7] p-3">{children}</div>
}
