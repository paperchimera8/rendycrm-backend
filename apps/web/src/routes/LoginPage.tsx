import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { zodResolver } from '@hookform/resolvers/zod'
import { useRouter } from '@tanstack/react-router'
import { login, setToken } from '../lib/api'

const schema = z.object({
  email: z.string().email('Введите корректный email'),
  password: z.string().min(4, 'Пароль должен содержать минимум 4 символа')
})

type FormValues = z.infer<typeof schema>

export function LoginPage() {
  const router = useRouter()
  const { register, handleSubmit, formState: { errors, isSubmitting }, setError } = useForm<FormValues>({
    resolver: zodResolver(schema)
  })

  const onSubmit = handleSubmit(async (values) => {
    try {
      const response = await login(values.email, values.password)
      setToken(response.token)
      router.navigate({ to: '/dialogs' })
    } catch (error) {
      setError('root', { message: error instanceof Error ? error.message : 'Не удалось выполнить вход' })
    }
  })

  return (
    <div className="min-h-screen bg-[radial-gradient(circle_at_top_left,_rgba(175,128,208,0.2),_transparent_32%),radial-gradient(circle_at_top_right,_rgba(128,137,168,0.2),_transparent_36%),#f7f7fb] px-4 py-12 text-[#292929]">
      <div className="mx-auto grid max-w-6xl gap-10 lg:grid-cols-[1.1fr_0.9fr]">
        <section className="rounded-[10px] border border-[#ebebeb] bg-white p-8">
          <p className="text-xs uppercase tracking-[0.35em] text-[#8e8e8e]">ОПЕРАТОРСКИЙ MVP</p>
          <h1 className="mt-6 text-5xl font-semibold leading-tight text-[#292929]">Диалоги, записи, отзывы и живой статус в одном интерфейсе.</h1>
          <p className="mt-6 max-w-lg text-base text-[#5e5e5e]">
            Минимальный стек: Go API, схема PostgreSQL, готовый к Redis runtime, React SPA, SSE-обновления, адаптеры Telegram и WhatsApp.
          </p>
          <div className="mt-10 grid gap-4 sm:grid-cols-3">
            <MetricCard value="2" label="подключенных канала" />
            <MetricCard value="1" label="активный оператор" />
            <MetricCard value="SSE" label="живой транспорт" />
          </div>
        </section>
        <section className="rounded-[10px] border border-[#ebebeb] bg-white p-8">
          <h2 className="text-2xl font-semibold text-[#292929]">Вход оператора</h2>
          <p className="mt-2 text-sm text-[#5e5e5e]">Введите email и пароль оператора.</p>
          <form className="mt-8 space-y-5" onSubmit={onSubmit}>
            <label className="block">
              <span className="mb-2 block text-xs uppercase tracking-[0.2em] text-[#8e8e8e]">Email</span>
              <input {...register('email')} className="w-full rounded-[10px] border border-[#ebebeb] bg-white px-4 py-3 outline-none ring-0 transition focus:border-[#8089a8]" />
              {errors.email && <p className="mt-2 text-sm text-red-600">{errors.email.message}</p>}
            </label>
            <label className="block">
              <span className="mb-2 block text-xs uppercase tracking-[0.2em] text-[#8e8e8e]">Пароль</span>
              <input type="password" {...register('password')} className="w-full rounded-[10px] border border-[#ebebeb] bg-white px-4 py-3 outline-none ring-0 transition focus:border-[#8089a8]" />
              {errors.password && <p className="mt-2 text-sm text-red-600">{errors.password.message}</p>}
            </label>
            {errors.root && <p className="rounded-[10px] border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{errors.root.message}</p>}
            <button data-testid="login-submit" disabled={isSubmitting} className="w-full rounded-[10px] bg-[#252525] px-5 py-3 text-sm font-medium uppercase tracking-[0.15em] text-[#fbfbfb] transition hover:bg-[#343434] disabled:opacity-70">
              {isSubmitting ? 'Входим...' : 'Открыть рабочее пространство'}
            </button>
          </form>
        </section>
      </div>
    </div>
  )
}

function MetricCard({ value, label }: { value: string; label: string }) {
  return (
    <div className="rounded-[10px] border border-[#ebebeb] bg-[#f7f7f7] p-4">
      <p className="text-3xl font-semibold text-[#292929]">{value}</p>
      <p className="mt-2 text-xs uppercase tracking-[0.2em] text-[#8e8e8e]">{label}</p>
    </div>
  )
}
