import { Outlet, Link, useRouter } from '@tanstack/react-router'
import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { apiUrl, clearToken, getToken, logout } from '../lib/api'
import { useMe } from '../lib/queries'
import { useUIStore } from '../stores/ui'

const navItems = [
  { to: '/dashboard', label: 'Дашборд' },
  { to: '/dialogs', label: 'Диалоги' },
  { to: '/slots', label: 'Слоты' },
  { to: '/reviews', label: 'Отзывы' },
  { to: '/analytics', label: 'Аналитика' },
  { to: '/settings', label: 'Настройки' }
]

export function AppShell() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const me = useMe()
  const toasts = useUIStore((state) => state.toasts)
  const removeToast = useUIStore((state) => state.removeToast)

  useEffect(() => {
    const token = getToken()
    if (!token) return
    const events = new EventSource(`${apiUrl('/events')}?token=${encodeURIComponent(token)}`)
    const refresh = () => {
      queryClient.invalidateQueries({ queryKey: ['dashboard'] })
      queryClient.invalidateQueries({ queryKey: ['conversations'] })
      queryClient.invalidateQueries({ queryKey: ['reviews'] })
      queryClient.invalidateQueries({ queryKey: ['analytics'] })
      queryClient.invalidateQueries({ queryKey: ['slot-editor'] })
      queryClient.invalidateQueries({ queryKey: ['week-slots'] })
      queryClient.invalidateQueries({ queryKey: ['available-slots'] })
      queryClient.invalidateQueries({ queryKey: ['bookings'] })
    }
    events.addEventListener('message.new', refresh)
    events.addEventListener('booking.updated', refresh)
    events.addEventListener('review.new', refresh)
    events.addEventListener('dashboard.updated', refresh)
    return () => events.close()
  }, [queryClient])

  const onLogout = async () => {
    try {
      await logout()
    } catch {
      // ignore logout failure and clear local state
    }
    clearToken()
    queryClient.clear()
    router.navigate({ to: '/login' })
  }

  return (
    <div className="min-h-screen text-[#292929]">
      <div className="fixed right-4 top-4 z-50 space-y-2" aria-live="polite" aria-atomic="false">
        {toasts.map((toast) => (
          <button
            key={toast.id}
            onClick={() => removeToast(toast.id)}
            role={toast.tone === 'error' ? 'alert' : 'status'}
            className={`block rounded-[10px] border px-4 py-3 text-left text-sm ${toast.tone === 'success' ? 'border-emerald-200 bg-emerald-50 text-emerald-800' : 'border-red-200 bg-red-50 text-red-800'}`}
          >
            {toast.text}
          </button>
        ))}
      </div>
      <div className="grid min-h-screen w-full grid-cols-1 lg:grid-cols-[232px_minmax(0,1fr)]">
        <aside className="border-b border-[#e8e4ef] bg-gradient-to-b from-[#faf8fc] to-[#f2eef8] p-4 lg:sticky lg:top-0 lg:h-screen lg:overflow-y-auto lg:border-b-0 lg:border-r lg:border-[#e8e4ef]">
          <nav className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-1">
            {navItems.map((item) => (
              <Link
                key={item.to}
                to={item.to}
                className="block rounded-lg border border-[#e5e0ed] bg-white/80 px-3 py-2 text-sm font-medium text-[#3d3852] transition-colors hover:border-[#a78bfa] hover:bg-white hover:shadow-[0_0_0_1px_rgba(139,92,246,0.15)]"
                activeProps={{ className: 'block rounded-lg border-l-4 border-l-[#8b5cf6] border-[#d4cce4] bg-white px-3 py-2 text-sm font-medium text-[#252525] shadow-sm' }}
              >
                {item.label}
              </Link>
            ))}
          </nav>
          <div className="mt-6 rounded-lg border border-[#e5e0ed] bg-white/70 p-3 text-sm backdrop-blur-sm">
            <p className="font-medium text-[#3d3852]">{me.data?.user?.name ?? '...'}</p>
            <p className="text-xs text-[#6e6a7a]">{me.data?.workspace?.name ?? 'Workspace'}</p>
            <button onClick={onLogout} className="mt-2 text-xs text-[#6e6a7a] hover:text-[#3d3852]">
              Выйти
            </button>
          </div>
        </aside>
        <main className="min-w-0 bg-[radial-gradient(circle_at_top_right,_rgba(175,128,208,0.13),_transparent_32%),radial-gradient(circle_at_top_left,_rgba(128,137,168,0.14),_transparent_35%),#f7f7fb] p-4 sm:p-6 xl:p-8">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
