// The editor's interface language. English is the source language and the default:
// the English text IS the key, so a string with no translation still reads correctly.
// Other languages are dictionaries in app/i18n/. app/i18n/i18n.test.ts fails when a
// string has no Danish, or when Danish text is left in the source.
//
// NUXT_PUBLIC_EDITOR_LOCALE picks the language. This is the language of the interface
// only, not of the content being edited.
import { da } from '~/i18n/da'

const DICTIONARIES: Record<string, Record<string, string>> = { da }

export type TParams = Record<string, string | number>

/** Translates `key` and fills `{name}` placeholders. Pure, so it can be tested. */
export function translate(locale: string, key: string, params?: TParams): string {
  const text = DICTIONARIES[locale]?.[key] ?? key
  if (!params) return text
  return text.replace(/\{(\w+)\}/g, (whole, name: string) => (name in params ? String(params[name]) : whole))
}

export function useT() {
  const locale = (useRuntimeConfig().public.editorLocale as string) || 'en'
  return { locale, t: (key: string, params?: TParams) => translate(locale, key, params) }
}
