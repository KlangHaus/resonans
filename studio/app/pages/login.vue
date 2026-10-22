<script setup lang="ts">
// Login for the editor. With OIDC configured, the button leaves for the
// identity provider (via the editor's own /auth/login); there is no password
// field here, because Resonans keeps no passwords (ADR 0001). In dev mode the
// guard never sends anyone here, and the page says so.
const { enabled, ready, refresh, signIn } = useAuth()
const route = useRoute()

// The route guard and the server's callback each say WHY someone is here, and
// each reason is its own message: an editor who cannot tell "the service is
// down" from "your login expired" will just click again and again.
const reason = computed(() => route.query.grund as string | undefined)
const next = computed(() => {
  const n = route.query.naeste
  return typeof n === 'string' && n.startsWith('/') ? n : '/'
})

onMounted(async () => {
  if (!ready.value) {
    try {
      await refresh()
    } catch {
      // Shown below as "could not reach"; nothing else to do here.
    }
  }
})
</script>

<template>
  <div class="lg">
    <div class="lg__card">
      <h1 class="lg__h">Log ind i Resonans</h1>

      <p v-if="ready && !enabled" class="lg__note">
        Der er ingen login-tjeneste konfigureret for dette miljø, så login er slået fra. Resonans kører i
        udviklingstilstand.
      </p>

      <template v-else>
        <p v-if="reason === 'utilgaengelig'" class="lg__warn">
          Vi kunne ikke få fat i login-tjenesten. Du er ikke logget ud — vi ved bare ikke, om du er logget ind.
        </p>
        <p v-else-if="reason === 'udloebet'" class="lg__warn">
          Login-forsøget udløb, eller blev startet i en anden fane. Prøv igen.
        </p>
        <p v-else-if="reason === 'afvist'" class="lg__warn">
          Login-tjenesten sagde nej, eller svaret kunne ikke bekræftes. Prøv igen — og sig til, hvis det bliver ved.
        </p>

        <button type="button" class="lg__btn" @click="signIn(next)">Log ind</button>
      </template>
    </div>
  </div>
</template>

<style scoped>
.lg {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  background: var(--rz-bg-subtle);
}
.lg__card {
  width: 100%;
  max-width: 360px;
  padding: 28px;
  border: 1px solid var(--rz-border);
  border-radius: 12px;
  background: var(--rz-bg);
}
.lg__h {
  margin: 0 0 18px;
  font-size: 18px;
}
.lg__btn {
  width: 100%;
  padding: 9px 12px;
  border: 0;
  border-radius: 7px;
  background: var(--rz-primary);
  color: #fff;
  font-weight: 600;
  cursor: pointer;
}
.lg__btn:disabled {
  opacity: 0.6;
  cursor: default;
}
.lg__warn,
.lg__note {
  margin: 0 0 14px;
  padding: 9px 11px;
  border-radius: 8px;
  font-size: 12px;
  background: color-mix(in srgb, #ffc107 16%, transparent);
  border: 1px solid color-mix(in srgb, #ffc107 40%, transparent);
  color: #7a5b00;
}
</style>
