import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach, beforeEach, vi } from 'vitest'
import { useUIStore } from '../stores/ui'

const resetUIState = () => {
  useUIStore.setState({
    selectedConversationId: null,
    composeDraft: '',
    dialogFilter: 'all',
    channelFilter: 'all',
    reviewFilter: 'all',
    toasts: []
  })
}

beforeEach(() => {
  localStorage.clear()
  resetUIState()
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  localStorage.clear()
  resetUIState()
})
