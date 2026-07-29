// useSpaces — the Studio space registry (Storyblok-model: a space = an isolated
// content-tenant). Architecturally a space IS a tenant (Beslutning K: tenant ≡
// org-id, client-supplied UUID) — so "creating a space" mints a fresh UUID and
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

function load(): Space[] {
  try {
    return JSON.parse(localStorage.getItem(LS_SPACES) || '[]') as Space[]
  } catch {
    return []
  }
}

export function useSpaces() {
  const cfg = useRuntimeConfig()
  const devTenant = (cfg.public.devTenant as string) || ''

  const spaces = useState<Space[]>('spaces', () => {
    const stored = load()
    // Seed: the configured dev-tenant is always present as the first space.
    if (devTenant && !stored.some((s) => s.id === devTenant)) {
      stored.unshift({ id: devTenant, name: 'KlangHaus', createdAt: new Date().toISOString() })
    }
    return stored
  })
  const activeId = useState<string>('activeSpace', () => localStorage.getItem(LS_ACTIVE) || '')

  function persist() {
    localStorage.setItem(LS_SPACES, JSON.stringify(spaces.value))
  }

  const active = computed(() => spaces.value.find((s) => s.id === activeId.value) || null)

  function enter(id: string) {
    activeId.value = id
    localStorage.setItem(LS_ACTIVE, id)
  }
  function leave() {
    activeId.value = ''
    localStorage.removeItem(LS_ACTIVE)
  }
  function create(name: string): Space {
    const s: Space = { id: crypto.randomUUID(), name: name.trim(), createdAt: new Date().toISOString() }
    spaces.value.push(s)
    persist()
    return s
  }

  return { spaces, active, activeId, enter, leave, create }
}
