import { create } from 'zustand'

type UIState = {
  selectedConversationId: string | null
  composeDraft: string
  dialogFilter: 'all' | 'new' | 'auto' | 'human' | 'booked' | 'closed'
  channelFilter: 'all' | 'telegram' | 'whatsapp'
  reviewFilter: 'all' | 'open' | 'resolved'
  toasts: Array<{ id: number; tone: 'success' | 'error'; text: string }>
  setSelectedConversationId: (id: string | null) => void
  setComposeDraft: (value: string) => void
  setDialogFilter: (value: 'all' | 'new' | 'auto' | 'human' | 'booked' | 'closed') => void
  setChannelFilter: (value: 'all' | 'telegram' | 'whatsapp') => void
  setReviewFilter: (value: 'all' | 'open' | 'resolved') => void
  pushToast: (tone: 'success' | 'error', text: string) => void
  removeToast: (id: number) => void
}

export const useUIStore = create<UIState>((set, get) => ({
  selectedConversationId: null,
  composeDraft: '',
  dialogFilter: 'all',
  channelFilter: 'all',
  reviewFilter: 'all',
  toasts: [],
  setSelectedConversationId: (selectedConversationId) => set({ selectedConversationId }),
  setComposeDraft: (composeDraft) => set({ composeDraft }),
  setDialogFilter: (dialogFilter) => set({ dialogFilter }),
  setChannelFilter: (channelFilter) => set({ channelFilter }),
  setReviewFilter: (reviewFilter) => set({ reviewFilter }),
  pushToast: (tone, text) => {
    const id = Date.now() + Math.random()
    let wasAdded = false
    set((state) => {
      if (state.toasts.some((toast) => toast.tone === tone && toast.text === text)) {
        return state
      }
      wasAdded = true
      return {
        toasts: [...state.toasts, { id, tone, text }]
      }
    })
    if (wasAdded && typeof window !== 'undefined') {
      window.setTimeout(() => {
        get().removeToast(id)
      }, tone === 'error' ? 4500 : 2500)
    }
  },
  removeToast: (id) =>
    set((state) => ({
      toasts: state.toasts.filter((toast) => toast.id !== id)
    }))
}))
