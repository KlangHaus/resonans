// The guard fails closed: a server it cannot ask is not "you are allowed".
import { beforeEach, describe, expect, it, vi } from 'vitest'

const ref = <T>(value: T) => ({ value })
const computed = <T>(get: () => T) => ({
  get value() {
    return get()
  },
})

const enabled = ref(false)
const user = ref<unknown>(null)
const ready = ref(false)
const refresh = vi.fn()
const navigateTo = vi.fn((to: unknown) => to)

vi.stubGlobal('defineNuxtRouteMiddleware', (fn: unknown) => fn)
vi.stubGlobal('useAuth', () => ({ enabled: computed(() => enabled.value), user, ready, refresh }))
vi.stubGlobal('navigateTo', navigateTo)
vi.stubGlobal('DEV_ONLY_ROUTES', ['/', '/organization', '/spaces/new'])

const guard = (await import('./auth.global')).default as unknown as (to: {
  path: string
  fullPath: string
}) => Promise<unknown>
const go = (path: string) => guard({ path, fullPath: path })

beforeEach(() => {
  vi.clearAllMocks()
  enabled.value = false
  user.value = null
  ready.value = true
  refresh.mockResolvedValue(null)
})

describe('auth guard', () => {
  it('never redirects from the login page itself', async () => {
    enabled.value = true
    expect(await go('/login')).toBeUndefined()
  })

  it('leaves dev mode alone, on every route', async () => {
    for (const path of ['/', '/content', '/spaces/new']) expect(await go(path)).toBeUndefined()
  })

  it('sends an unreachable server to /login with its own reason', async () => {
    ready.value = false
    refresh.mockRejectedValue(new Error('down'))
    expect(await go('/content')).toEqual({ path: '/login', query: { reason: 'unreachable', next: '/content' } })
  })

  it('sends nobody-logged-in to /login, keeping the destination', async () => {
    enabled.value = true
    expect(await go('/content')).toEqual({ path: '/login', query: { next: '/content' } })
  })

  it.each(['/', '/organization', '/spaces/new'])('sends a logged-in editor from %s to the content', async (path) => {
    enabled.value = true
    user.value = { sub: 'u' }
    expect(await go(path)).toBe('/dashboard')
  })

  it('lets a logged-in editor through elsewhere', async () => {
    enabled.value = true
    user.value = { sub: 'u' }
    expect(await go('/content')).toBeUndefined()
  })
})
