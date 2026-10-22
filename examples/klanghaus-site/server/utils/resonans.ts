// Resonans' delivery API, server-side. The tenant and the token are set here and
// nowhere else: the browser only ever sees finished HTML.

export interface Component {
  _type: string
  _key?: string
  [field: string]: unknown
}

export interface Envelope {
  id: string
  type: string
  slug: string
  locale: string
  publishedAt?: string
  data: Record<string, unknown>
  components?: Component[]
}

export interface PostSummary {
  id: string
  slug: string
  title: string
  summary?: string
  publishedAt?: string
  thumbnail?: { url: string; alt?: string }
  [k: string]: unknown
}

function client() {
  const c = useRuntimeConfig()
  const headers: Record<string, string> = { 'X-Scope-OrgID': String(c.resonansTenant) }
  if (c.resonansDeliveryToken) headers.Authorization = `Bearer ${c.resonansDeliveryToken}`
  return { base: String(c.resonansUrl).replace(/\/$/, ''), headers }
}

/** One published document, or null when it does not exist (404). */
export async function getDocument(type: string, slug: string): Promise<Envelope | null> {
  const { base, headers } = client()
  try {
    return await $fetch<Envelope>(`${base}/delivery/v1/${type}/${encodeURIComponent(slug)}`, {
      query: { locale: 'da' },
      headers,
    })
  } catch (e: unknown) {
    const status = (e as { statusCode?: number })?.statusCode ?? (e as { response?: { status?: number } })?.response?.status
    if (status === 404) return null
    throw e
  }
}

/** Published documents of a type, newest first (Resonans orders them). */
export async function listDocuments<T = PostSummary>(type: string): Promise<T[]> {
  const { base, headers } = client()
  const res = await $fetch<{ items?: T[] }>(`${base}/delivery/v1/${type}`, { query: { locale: 'da' }, headers })
  return res.items ?? []
}
