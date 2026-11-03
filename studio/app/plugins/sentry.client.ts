// Sentry (SaaS, EU region) for the Studio SPA: error capture + the User Feedback
// widget behind "Report a problem" in the Help & Inspiration drop-up. The DSN is publishable (not a
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

  const { t } = useT()

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
        triggerLabel: t('Report a problem'),
        formTitle: t('Report a problem'),
        submitButtonLabel: t('Send report'),
        cancelButtonLabel: t('Cancel'),
        messageLabel: t('What happened?'),
        messagePlaceholder: t('Describe the problem: what did you try, and what happened instead?'),
        successMessageText: t('Thanks! The report has been sent.'),
      }),
    ],
  })

  return {
    provide: {
      // Opens the Sentry feedback form from code (Help & Inspiration → Report a problem).
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
