// Logged in, the token decides the tenant: one space, nothing to pick, and the
// client-side space registry must not get a say. Dev mode keeps its registry.
import { beforeEach, describe, expect, it, vi } from 'vitest'

const ref = <T>(value: T) => ({ value })
const computed = <T>(get: () => T) => ({
  get value() {
    return get()
  },
})

const authEnabled = ref(false)
const states = new Map<string, { value: unknown }>()

vi.stubGlobal('computed', computed)
vi.stubGlobal('useAuth', () => ({ enabled: authEnabled }))
vi.stubGlobal('useRuntimeConfig', () => ({ public: { devTenant: 'dev-tenant' } }))
vi.stubGlobal('useState', (key: string, init: () => unknown) => {
  if (!states.has(key)) states.set(key, ref(init()))
  return states.get(key)
})

const { useSpaces, DEV_ONLY_ROUTES } = await import('./useSpaces')

beforeEach(() => {
  authEnabled.value = false
  states.clear()
  localStorage.clear()
})

describe('logged in', () => {
  beforeEach(() => {
    authEnabled.value = true
  })

  it('always has the one space, without entering it', () => {
    expect(useSpaces().active.value?.name).toBe('Content')
  })

  it('ignores a space left in this browser from dev mode', () => {
    localStorage.setItem('resonans.activeSpace', 'stale-tenant')
    expect(useSpaces().activeId.value).toBe('session')
  })

  it('stays in the space after leaving it', () => {
    const spaces = useSpaces()
    spaces.leave()
    expect(spaces.active.value?.name).toBe('Content')
  })
})

describe('dev mode', () => {
  it('starts with no active space', () => {
    expect(useSpaces().active.value).toBeNull()
  })

  it('seeds the dev tenant under a neutral name', () => {
    expect(useSpaces().spaces.value).toEqual([expect.objectContaining({ id: 'dev-tenant', name: 'Default' })])
  })

  it('enters and leaves, and remembers across a reload', () => {
    const spaces = useSpaces()
    spaces.enter('dev-tenant')
    expect(spaces.active.value?.id).toBe('dev-tenant')
    expect(localStorage.getItem('resonans.activeSpace')).toBe('dev-tenant')
    spaces.leave()
    expect(spaces.active.value).toBeNull()
    expect(localStorage.getItem('resonans.activeSpace')).toBeNull()
  })

  it('creates a space with its own tenant id', () => {
    const spaces = useSpaces()
    const created = spaces.create('  Marketing ')
    expect(created.name).toBe('Marketing')
    expect(created.id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
  })
})

it('names the routes only dev mode has', () => {
  expect(DEV_ONLY_ROUTES).toEqual(['/', '/organization', '/spaces/new'])
})
