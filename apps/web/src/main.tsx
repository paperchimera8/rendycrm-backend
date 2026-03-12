import React from 'react'
import ReactDOM from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { AppErrorBoundary } from './components/AppErrorBoundary'
import { router } from './router'
import './styles.css'

const queryClient = new QueryClient()
;(window as Window & { __rendyBooted?: boolean }).__rendyBooted = true

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <AppErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} context={{ queryClient }} />
      </QueryClientProvider>
    </AppErrorBoundary>
  </React.StrictMode>
)
