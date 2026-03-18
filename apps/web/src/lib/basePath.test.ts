import { describe, expect, it } from 'vitest'
import { normalizePath, resolveAppBasePath } from './basePath'

describe('basePath helpers', () => {
  it('normalizes empty values to root', () => {
    expect(normalizePath(undefined)).toBe('/')
    expect(normalizePath('')).toBe('/')
    expect(normalizePath('/')).toBe('/')
  })

  it('keeps nested app paths normalized', () => {
    expect(normalizePath('app')).toBe('/app')
    expect(normalizePath('/app/')).toBe('/app')
    expect(normalizePath('///app///nested///')).toBe('/app/nested')
  })

  it('prefers build-time app base when runtime config falls back to root', () => {
    expect(resolveAppBasePath('/', '/app/')).toBe('/app')
    expect(resolveAppBasePath(undefined, '/app/')).toBe('/app')
  })

  it('respects runtime override when it is explicit', () => {
    expect(resolveAppBasePath('/crm', '/app/')).toBe('/crm')
    expect(resolveAppBasePath('/crm', undefined)).toBe('/crm')
  })
})
