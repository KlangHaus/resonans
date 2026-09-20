<script setup lang="ts">
// Login for the Studio. Only reachable when an auth service is configured; in
// dev mode the guard never sends anyone here, and the page says so rather than
// presenting a form that cannot work.
const { enabled, signIn, token } = useAuth()
const route = useRoute()

const email = ref('')
const password = ref('')
const busy = ref(false)
const error = ref('')

// `grund=utilgaengelig` is set by the route guard when the session lookup threw.
// It is a DIFFERENT message from "wrong password": an editor who cannot tell
// "the service is down" from "I typed it wrong" will retype it five times.
const unreachable = computed(() => route.query.grund === 'utilgaengelig')

async function submit() {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    const u = await signIn(email.value.trim(), password.value)
    if (!u) {
      error.value = 'Login lykkedes, men der kom ingen bruger tilbage. Prøv igen, eller sig til.'
      return
    }
    // Mint the JWT here rather than on the first editor call: a failure now is
    // readable ("logget ind, men kunne ikke hente adgangsbevis") instead of an
    // unexplained 401 on the first thing the editor clicks.
    await token(true)
    await navigateTo((route.query.naeste as string) || '/')
  } catch (e: unknown) {
    const status = (e as { statusCode?: number; status?: number })?.statusCode ?? (e as { status?: number })?.status
    error.value =
      status === 401
        ? 'Forkert e-mail eller kodeord.'
        : status === 429
          ? 'For mange forsøg. Vent et øjeblik, og prøv igen.'
          : 'Kunne ikke nå login-tjenesten. Prøv igen om lidt — og sig til, hvis den bliver ved.'
  } finally {
    busy.value = false
    password.value = ''
  }
}
</script>

<template>
  <div class="lg">
    <div class="lg__card">
      <h1 class="lg__h">Log ind i Resonans</h1>

      <p v-if="!enabled" class="lg__note">
        Der er ingen login-tjeneste konfigureret for dette miljø, så login er slået fra. Studio kører i
        udviklingstilstand.
      </p>

      <template v-else>
        <p v-if="unreachable" class="lg__warn">
          Vi kunne ikke få fat i login-tjenesten. Du er ikke logget ud — vi ved bare ikke, om du er logget ind.
        </p>

        <form @submit.prevent="submit">
          <label class="lg__l" for="email">E-mail</label>
          <input id="email" v-model="email" type="email" autocomplete="username" required class="lg__i" />

          <label class="lg__l" for="password">Kodeord</label>
          <input
            id="password"
            v-model="password"
            type="password"
            autocomplete="current-password"
            required
            class="lg__i"
          />

          <p v-if="error" class="lg__err">{{ error }}</p>

          <button type="submit" class="lg__btn" :disabled="busy">
            {{ busy ? 'Logger ind…' : 'Log ind' }}
          </button>
        </form>
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
.lg__l {
  display: block;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--rz-text-muted);
  margin-bottom: 4px;
}
.lg__i {
  width: 100%;
  padding: 8px 10px;
  margin-bottom: 14px;
  border: 1px solid var(--rz-border);
  border-radius: 7px;
  background: var(--rz-bg);
  color: var(--rz-text);
  font: inherit;
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
.lg__err {
  margin: 0 0 12px;
  font-size: 12.5px;
  color: #b3261e;
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
