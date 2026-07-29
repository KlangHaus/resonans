// Sentry (SaaS EU — Beslutning: error-tracking = Sentry EU-region) for the
// Studio SPA: error capture + the User Feedback widget behind "Rapportér et
// problem" in the Help & Inspiration drop-up. The DSN is publishable (not a
// secret); override per environment via NUXT_PUBLIC_SENTRY_DSN, empty = Sentry
// disabled entirely (e.g. local dev without noise).
//
// NOTE: this is the BROWSER SDK for our actual stack (Nuxt SPA). The Sentry
// wizard's Hono/Cloudflare snippet does not apply here — the Go backend gets
// sentry-go as its own increment.
import * as Sentry from '@sentry/vue'

export default defineNuxtPlugin((nuxtApp) => {
  const cfg = useRuntimeConfig()
  const dsn = (cfg.public.sentryDsn as string) || ''
  if (!dsn) return

  Sentry.init({
    app: nuxtApp.vueApp,
    dsn,
    environment: (cfg.public.sentryEnv as string) || 'staging',
    integrations: [
      // autoInject:false — we open the form from the Help drop-up, no floating button.
      Sentry.feedbackIntegration({
        autoInject: false,
        showBranding: false,
        colorScheme: 'light',
        triggerLabel: 'Rapportér et problem',
        formTitle: 'Rapportér et problem',
        submitButtonLabel: 'Send rapport',
        cancelButtonLabel: 'Annullér',
        messageLabel: 'Hvad skete der?',
        messagePlaceholder: 'Beskriv problemet — hvad prøvede du, og hvad skete der i stedet?',
        successMessageText: 'Tak! Rapporten er sendt.',
      }),
    ],
  })

  return {
    provide: {
      // Åbn Sentry-feedback-formen programmatisk (Help & Inspiration → Rapportér et problem).
      reportProblem: async () => {
        const feedback = Sentry.getFeedback()
        if (!feedback) return
        const form = await feedback.createForm()
        form.appendToDom()
        form.open()
      },
    },
  }
})
