// useResonans — the typed client for the Resonans backend (/api/v1). The Studio
// is schema-driven: it fetches the registry and renders nav + forms from it, so
// there are no hardcoded content types here.

export type FieldType =
  | 'string'
  | 'text'
  | 'richtext'
  | 'number'
  | 'boolean'
  | 'asset'
  | 'reference'
  | 'blocks'
  | 'array'

export interface Field {
  name: string
  type: FieldType
  i18n?: boolean
  required?: boolean
  allow?: string[]
  ref?: string
  of?: Field[]
}

export interface DocType {
  type: string
  fields: Field[]
  list?: { columns?: string[] }
}

export interface BlockType {
  name: string
  fields: Field[]
}

export interface Schema {
  docTypes: DocType[]
  blocks: BlockType[]
}

export interface Document {
  id: string
  tenantId: string
  dataset: string
  type: string
  slug: string
  title: string
  body: Record<string, unknown>
  status: 'draft' | 'published'
  publishedAt?: string
  createdAt: string
  updatedAt: string
}

export interface PreviewSettings {
  previewUrl: string
  // The ONLY origin the Studio exchanges postMessages with ("" = not usable).
  previewOrigin: string
  // URL stored AND preview tokens can be signed on the server.
  previewReady: boolean
}

export interface CreateInput {
  type: string
  slug: string
  title: string
  body?: Record<string, unknown>
}

export function useResonans() {
  const cfg = useRuntimeConfig()
  const base = (cfg.public.apiBase as string) || ''
  const devTenant = (cfg.public.devTenant as string) || ''
  const { activeId } = useSpaces()
  const auth = useAuth()

  function headers(extra: Record<string, string> = {}, jwt?: string | null): Record<string, string> {
    const h: Record<string, string> = { ...extra }
    // 🔴 THE TWO WAYS IN ARE MUTUALLY EXCLUSIVE, AND THE SERVER AGREES.
    // With JWT configured, `auth.Middleware` reads ONLY the Bearer token and
    // never looks at X-Dev-Tenant — so sending both would not be belt and
    // braces, it would be a header that looks like it does something. With no
    // auth service configured we are in dev mode and the tenant header is the
    // only way in.
    if (jwt) {
      h.Authorization = `Bearer ${jwt}`
      return h
    }
    // Tenant = the ACTIVE SPACE (space ≡ tenant, Beslutning K), falling back to
    // the configured dev-tenant. Dev-mode only: in prod the session/JWT supplies
    // the tenant and this header is ignored server-side.
    const tenant = activeId.value || devTenant
    if (tenant) h['X-Dev-Tenant'] = tenant
    return h
  }

  // No explicit `Promise<T>` return type: $fetch resolves to
  // TypedInternalResponse<…, T, "get">, which is T for these plain URLs but is
  // not assignable to Promise<T> by name. Inferring keeps the callers' types
  // without casting away the difference.
  async function req<T>(path: string, opts: Record<string, unknown> = {}) {
    // `headers` goes LAST: spread after `...opts`, a caller's own headers
    // (e.g. Content-Type) replaced the merged set and dropped the tenant header,
    // so every write with a body got 403 while every read worked.
    const send = (jwt: string | null) =>
      $fetch<T>(`${base}${path}`, {
        credentials: 'include',
        ...opts,
        headers: headers(opts.headers as Record<string, string>, jwt),
      })
    const jwt = await auth.token()
    try {
      return await send(jwt)
    } catch (e: unknown) {
      // 🔴 EXACTLY ONE RETRY, AND ONLY FOR 401.
      // A JWT expires while the session is still valid, so one silent re-mint
      // is the difference between "your work is gone" and nothing at all. But
      // a retry loop against an auth service that has stopped saying yes is a
      // way to hammer it while showing the editor nothing: if the second call
      // is 401 too, the error surfaces and the route guard sends them to log
      // in. A 403 is not retried — that is an answer, not an expiry.
      const status = (e as { statusCode?: number; status?: number })?.statusCode ?? (e as { status?: number })?.status
      if (status !== 401 || !auth.enabled.value) throw e
      const fresh = await auth.token(true)
      if (!fresh) throw e
      return await send(fresh)
    }
  }

  return {
    schema: () => req<Schema>('/api/v1/schema'),
    listDocuments: (type: string, status?: string) =>
      req<{ documents: Document[] }>(
        `/api/v1/documents?type=${encodeURIComponent(type)}${status ? `&status=${status}` : ''}`,
      ).then((r) => r.documents),
    getDocument: (id: string) => req<Document>(`/api/v1/documents/${id}`),
    createDocument: (input: CreateInput) =>
      req<Document>('/api/v1/documents', {
        method: 'POST',
        body: input,
        headers: { 'Content-Type': 'application/json' },
      }),
    updateDocument: (id: string, body: { type?: string; title?: string; body?: Record<string, unknown> }) =>
      req<Document>(`/api/v1/documents/${id}`, {
        method: 'PUT',
        body,
        headers: { 'Content-Type': 'application/json' },
      }),
    publishDocument: (id: string) =>
      req<Document>(`/api/v1/documents/${id}/publish`, { method: 'POST' }),
    listAssets: () =>
      req<{ assets: { id: string; filename: string; kind: string; mime: string; sizeBytes: number; width?: number; height?: number; url: string }[] }>(
        '/api/v1/assets',
      ).then((r) => r.assets),
    uploadAsset: (file: File) => {
      const fd = new FormData()
      fd.append('file', file)
      return req('/api/v1/assets', { method: 'POST', body: fd })
    },
    getPreviewSettings: () => req<PreviewSettings>('/api/v1/settings/preview'),
    putPreviewSettings: (previewUrl: string) =>
      req<PreviewSettings>('/api/v1/settings/preview', {
        method: 'PUT',
        body: { previewUrl },
        headers: { 'Content-Type': 'application/json' },
      }),
    previewToken: (type: string, slug: string) =>
      req<{ token: string; expiresInSeconds: number }>('/api/v1/preview-token', {
        method: 'POST',
        body: { type, slug },
        headers: { 'Content-Type': 'application/json' },
      }),
    // Delivery envelope of an UNSAVED body — what the preview iframe renders.
    previewEnvelope: (input: { type: string; slug: string; title: string; locale?: string; body: Record<string, unknown> }) =>
      req<Record<string, unknown>>('/api/v1/preview/envelope', {
        method: 'POST',
        body: input,
        headers: { 'Content-Type': 'application/json' },
      }),
    getSettings: () =>
      req<{ enabledTypes: string[]; allTypes: string[] }>('/api/v1/settings'),
    putSettings: (enabledTypes: string[]) =>
      req<{ enabledTypes: string[]; allTypes: string[] }>('/api/v1/settings', {
        method: 'PUT',
        body: { enabledTypes },
        headers: { 'Content-Type': 'application/json' },
      }),
  }
}
