// useSpaces — the Studio space registry (Storyblok-model: a space = an isolated
// content-tenant). Architecturally a space IS a tenant (tenant ≡ org-id,
// client-supplied UUID) — so "creating a space" mints a fresh UUID and
// RLS isolates its content automatically; no backend change needed.
//
// Dev/staging (no auth): the registry lives in localStorage, seeded with the
// configured dev-tenant as the first space. In prod-Studio the space list will
// come from the user's auth-org memberships (BetterAuth) instead — this
// composable is the seam where that swap happens.
export interface Space {
  id: string // tenant UUID (RLS key)
  name: string
  createdAt: string
}

const LS_SPACES = 'resonans.spaces'
const LS_ACTIVE = 'resonans.activeSpace'

// crypto.randomUUID is [SecureContext]-gated and MISSING on WG-internal staging
// (plain HTTP) — clicking "Create" would throw silently (#25). getRandomValues is
// NOT gated, so fall back to a manual RFC 4122 v4 when randomUUID is absent.
function uuidv4(): string {
  if (typeof crypto.randomUUID === 'function') return crypto.randomUUID()
  const b = new Uint8Array(16)
  crypto.getRandomValues(b)
  b[6] = (b[6]! & 0x0f) | 0x40 // version 4
  b[8] = (b[8]! & 0x3f) | 0x80 // variant 10xx
  const h = Array.from(b, (x) => x.toString(16).padStart(2, '0'))
  return `${h.slice(0, 4).join('')}-${h.slice(4, 6).join('')}-${h.slice(6, 8).join('')}-${h.slice(8, 10).join('')}-${h.slice(10).join('')}`
}

function load(): Space[] {
  try {
    return JSON.parse(localStorage.getItem(LS_SPACES) || '[]') as Space[]
  } catch {
    return []
  }
}

// Logged in, the token decides the tenant, so there is one space and nothing to
// pick. These pages only make sense in dev mode, where the tenant is a choice.
export const DEV_ONLY_ROUTES = ['/', '/organization', '/spaces/new']
const SESSION_SPACE: Space = { id: 'session', name: 'Content', createdAt: '' }

export function useSpaces() {
  const auth = useAuth()
  const cfg = useRuntimeConfig()
  const devTenant = (cfg.public.devTenant as string) || ''

  const spaces = useState<Space[]>('spaces', () => {
    const stored = load()
    // Seed: the configured dev-tenant is always present as the first space.
    if (devTenant && !stored.some((s) => s.id === devTenant)) {
      stored.unshift({ id: devTenant, name: 'Default', createdAt: new Date().toISOString() })
    }
    return stored
  })
  const storedActiveId = useState<string>('activeSpace', () => localStorage.getItem(LS_ACTIVE) || '')
  const activeId = computed(() => (auth.enabled.value ? SESSION_SPACE.id : storedActiveId.value))

  function persist() {
    localStorage.setItem(LS_SPACES, JSON.stringify(spaces.value))
  }

  const active = computed(() =>
    auth.enabled.value ? SESSION_SPACE : spaces.value.find((s) => s.id === activeId.value) || null,
  )

  function enter(id: string) {
    storedActiveId.value = id
    localStorage.setItem(LS_ACTIVE, id)
  }
  function leave() {
    storedActiveId.value = ''
    localStorage.removeItem(LS_ACTIVE)
  }
  function create(name: string): Space {
    const s: Space = { id: uuidv4(), name: name.trim(), createdAt: new Date().toISOString() }
    spaces.value.push(s)
    persist()
    return s
  }

  return { spaces, active, activeId, enter, leave, create }
}
