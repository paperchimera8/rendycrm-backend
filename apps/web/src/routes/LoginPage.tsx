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
      router.navigate({ to: '/' })
    } catch (error) {
      setError('root', { message: error instanceof Error ? error.message : 'Не удалось выполнить вход' })
    }
  })

  return (
    <div className="min-h-screen bg-[radial-gradient(circle_at_top_right,_rgba(175,128,208,0.15),_transparent_28%),radial-gradient(circle_at_top_left,_rgba(128,137,168,0.16),_transparent_34%),linear-gradient(180deg,_#faf8fc_0%,_#f4f1f8_48%,_#f7f7fb_100%)] px-4 py-8 text-[#292929] sm:px-6 sm:py-10 lg:pl-0">
      <div className="mx-auto grid min-h-[calc(100vh-4rem)] max-w-6xl items-stretch gap-6 lg:mx-0 lg:max-w-[min(96vw,92rem)] lg:grid-cols-[minmax(0,1.08fr)_minmax(26rem,34rem)] lg:pr-6">
        <section className="relative overflow-hidden rounded-[28px] border border-[#e8e4ef] bg-gradient-to-br from-[#faf8fc] via-[#f6f1fb] to-white p-8 shadow-[0_20px_80px_rgba(94,74,123,0.08)] sm:p-10 lg:rounded-l-none lg:border-l-0 lg:pl-10 lg:pr-12 xl:pl-12 xl:pr-16">
          <div className="absolute inset-y-0 right-0 hidden w-40 bg-[radial-gradient(circle_at_center,_rgba(139,92,246,0.16),_transparent_68%)] lg:block" aria-hidden />
          <div className="relative flex h-full max-w-[42rem] flex-col justify-between lg:ml-auto lg:mr-6 xl:mr-10">
            <div>
              <p className="max-w-[12ch] text-[clamp(1.7rem,3vw,2.75rem)] font-semibold leading-[0.98] tracking-[-0.04em] text-[#4a4260]">
                Клиенты пишут в Telegram и WhatsApp.
              </p>
              <p className="mt-3 max-w-[11ch] text-[clamp(1.7rem,3vw,2.75rem)] font-semibold leading-[0.98] tracking-[-0.04em] text-[#4a4260]">
                Оператор работает в одном окне.
              </p>
              <p className="mt-8 max-w-xl text-base leading-7 text-[#655d76] sm:text-lg">
                Диалоги, записи, слоты и статусы без переключения между чатами и сервисами.
              </p>
            </div>

            <div className="relative mt-10 grid gap-3 sm:grid-cols-3">
              <ValuePoint title="Единый inbox" text="Новые сообщения, ответы и статус обращения в одном потоке." />
              <ValuePoint title="Записи и слоты" text="Подтверждение, перенос и свободные окна без лишних вкладок." />
              <ValuePoint title="Живые статусы" text="Оператор сразу видит, что изменилось в работе с клиентом." />
            </div>
          </div>
        </section>

        <section className="flex items-center">
          <div className="w-full rounded-[28px] border border-[#e8e4ef] bg-white/90 p-8 shadow-[0_20px_80px_rgba(55,44,82,0.08)] backdrop-blur-sm sm:p-10">
            <h2 className="text-3xl font-semibold tracking-[-0.03em] text-[#2f2940]">Вход в рабочее пространство</h2>
            <p className="mt-3 text-sm leading-6 text-[#6f687f]">Введите email и пароль, чтобы открыть CRM.</p>

            <form className="mt-8 space-y-5" onSubmit={onSubmit}>
            <label className="block">
              <span className="mb-2 block text-xs font-semibold uppercase tracking-[0.2em] text-[#8e8e8e]">Email</span>
              <input
                {...register('email')}
                className="w-full rounded-[16px] border border-[#e5e0ed] bg-[#fcfbfe] px-4 py-3.5 text-[#292929] outline-none ring-0 transition placeholder:text-[#b3acbf] focus:border-[#8b5cf6] focus:bg-white"
                placeholder="operator@rendycrm.local"
              />
              {errors.email && <p className="mt-2 text-sm text-red-600">{errors.email.message}</p>}
            </label>
            <label className="block">
              <span className="mb-2 block text-xs font-semibold uppercase tracking-[0.2em] text-[#8e8e8e]">Пароль</span>
              <input
                type="password"
                {...register('password')}
                className="w-full rounded-[16px] border border-[#e5e0ed] bg-[#fcfbfe] px-4 py-3.5 text-[#292929] outline-none ring-0 transition placeholder:text-[#b3acbf] focus:border-[#8b5cf6] focus:bg-white"
                placeholder="Введите пароль"
              />
              {errors.password && <p className="mt-2 text-sm text-red-600">{errors.password.message}</p>}
            </label>
            {errors.root && <p className="rounded-[16px] border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{errors.root.message}</p>}
              <button
                data-testid="login-submit"
                disabled={isSubmitting}
                className="w-full rounded-[16px] bg-[#30293f] px-5 py-3.5 text-sm font-semibold text-[#fbfbfb] transition hover:bg-[#3d3451] disabled:opacity-70"
              >
                {isSubmitting ? 'Входим...' : 'Войти'}
              </button>
            </form>

            <p className="mt-6 text-xs leading-5 text-[#9389a7]">Используйте рабочий email и пароль мастера.</p>
          </div>
        </section>
      </div>
    </div>
  )
}

function ValuePoint({ title, text }: { title: string; text: string }) {
  return (
    <div className="rounded-[20px] border border-[#e6e0f0] bg-white/75 p-5 backdrop-blur-sm">
      <p className="text-sm font-semibold text-[#3a3250]">{title}</p>
      <p className="mt-2 text-sm leading-6 text-[#6c6480]">{text}</p>
    </div>
  )
}
