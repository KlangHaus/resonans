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

  function headers(extra: Record<string, string> = {}): Record<string, string> {
    const h: Record<string, string> = { ...extra }
    // Tenant = the ACTIVE SPACE (space ≡ tenant, Beslutning K), falling back to
    // the configured dev-tenant. Dev-mode only: in prod the session/JWT supplies
    // the tenant and this header is ignored server-side.
    const tenant = activeId.value || devTenant
    if (tenant) h['X-Dev-Tenant'] = tenant
    return h
  }

  function req<T>(path: string, opts: Record<string, unknown> = {}): Promise<T> {
    return $fetch<T>(`${base}${path}`, {
      credentials: 'include',
      headers: headers(opts.headers as Record<string, string>),
      ...opts,
    })
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
