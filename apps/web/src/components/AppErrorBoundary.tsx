import { Component, type ErrorInfo, type ReactNode } from 'react'
import { clearToken } from '../lib/api'

type Props = {
  children: ReactNode
}

type State = {
  hasError: boolean
  message: string
}

export class AppErrorBoundary extends Component<Props, State> {
  state: State = {
    hasError: false,
    message: ''
  }

  static getDerivedStateFromError(error: Error): State {
    return {
      hasError: true,
      message: error.message || 'Неизвестная ошибка интерфейса'
    }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('AppErrorBoundary', error, info)
  }

  private onReset = () => {
    clearToken()
    this.setState({ hasError: false, message: '' })
    window.location.href = '/login'
  }

  render() {
    if (!this.state.hasError) {
      return this.props.children
    }

    return (
      <div className="grid min-h-screen place-items-center bg-[radial-gradient(circle_at_top_left,_rgba(175,128,208,0.2),_transparent_32%),radial-gradient(circle_at_top_right,_rgba(128,137,168,0.2),_transparent_36%),#f7f7fb] p-6 text-[#292929]">
        <div className="w-full max-w-2xl rounded-[10px] border border-red-200 bg-white p-8">
          <p className="text-xs uppercase tracking-[0.3em] text-red-500">Ошибка интерфейса</p>
          <h1 className="mt-4 text-3xl font-semibold text-[#292929]">Интерфейс не удалось отрисовать.</h1>
          <p className="mt-3 text-sm text-[#5e5e5e]">Вместо белого экрана приложение теперь показывает состояние ошибки напрямую. Сбросьте сессию и откройте рабочее пространство заново.</p>
          <div className="mt-6 rounded-[10px] border border-red-200 bg-red-50 p-4 text-sm text-red-700">
            {this.state.message}
          </div>
          <div className="mt-6 flex flex-wrap gap-3">
            <button
              onClick={this.onReset}
              className="rounded-[10px] bg-[#252525] px-5 py-3 text-sm font-medium uppercase tracking-[0.15em] text-[#fbfbfb] transition hover:bg-[#343434]"
            >
              Сбросить сессию
            </button>
            <button
              onClick={() => window.location.reload()}
              className="rounded-[10px] border border-[#ebebeb] bg-[#f7f7f7] px-5 py-3 text-sm font-medium uppercase tracking-[0.15em] text-[#292929] transition-colors hover:bg-[#ebebeb]"
            >
              Перезагрузить приложение
            </button>
          </div>
        </div>
      </div>
    )
  }
}
