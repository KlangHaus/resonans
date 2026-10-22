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
  // Absent for a live document; set while it is in the bin.
  deletedAt?: string
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

  function headers(extra: Record<string, string> = {}): Record<string, string> {
    const h: Record<string, string> = { ...extra }
    // 🔴 THE TWO WAYS IN ARE MUTUALLY EXCLUSIVE, AND THE SERVER AGREES.
    // Logged in (OIDC), every call goes through the editor's own server at
    // /proxy, which adds the bearer token there — this page never holds one,
    // and the proxy forwards no tenant header. With no OIDC configured we are
    // in dev mode and the tenant header is the only way in.
    if (auth.enabled.value) return h
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
    const prefix = auth.enabled.value ? '/proxy' : base
    try {
      return await $fetch<T>(`${prefix}${path}`, {
        credentials: 'include',
        ...opts,
        headers: headers(opts.headers as Record<string, string>),
      })
    } catch (e: unknown) {
      // 🔴 401 MEANS THE SESSION IS GONE, NOT "TRY AGAIN".
      // The editor's server already refreshes the token when it can; a 401
      // from /proxy is what is left when it could not. Show the login page,
      // with this page as the way back — never jump straight to the provider:
      // a call that 401s ON the login page would then loop (measured: it did).
      // A 403 is not a login problem — it is the role answering — and
      // surfaces as the error it is.
      const status = (e as { statusCode?: number; status?: number })?.statusCode ?? (e as { status?: number })?.status
      if (status === 401 && auth.enabled.value && window.location.pathname !== '/login') {
        auth.user.value = null
        await navigateTo({ path: '/login', query: { naeste: window.location.pathname + window.location.search } })
      }
      throw e
    }
  }

  return {
    schema: () => req<Schema>('/api/v1/schema'),
    // The bin is a separate axis from status, so it is a separate argument:
    // folding it into `status` would make "published" and "deleted" look like
    // alternatives when a deleted document keeps the status it had.
    deleteDocument: (id: string) => req<Document>(`/api/v1/documents/${id}`, { method: 'DELETE' }),
    restoreDocument: (id: string) => req<Document>(`/api/v1/documents/${id}/restore`, { method: 'POST' }),
    listDeletedDocuments: (type: string) =>
      req<{ documents: Document[] }>(
        `/api/v1/documents?type=${encodeURIComponent(type)}&deleted=only`,
      ).then((r) => r.documents),
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
