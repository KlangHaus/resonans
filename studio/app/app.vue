<script setup lang="ts">
// Studio shell: a persistent dark rail (My Spaces / My organization on top; Help
// & inspiration and My account at the bottom) + a contextual light panel with the
// doc-type nav ONLY when inside a space.
const { t } = useT()
const route = useRoute()
const { active, leave } = useSpaces()
const { schema } = useResonans()

// The doc-type panel shows only inside a space (not on landing/org/create).
const SHELL_ROUTES = ['/', '/organization', '/spaces/new']
const inSpace = computed(
  () => !!active.value && !SHELL_ROUTES.includes(route.path) && route.path !== '/login',
)

const { data: sch, error, refresh } = await useAsyncData('schema', () => schema())
// Space-switch → the schema (and its content-profile filtering) must refetch.
watch(() => active.value?.id, () => refresh())

function backToSpaces() {
  leave()
  navigateTo('/')
}

const auth = useAuth()
// Initials from the signed-in address; '?' when there is no session, so the
// avatar never shows someone else's letters from a previous login.
const initials = computed(() => {
  const mail = auth.user.value?.email || ''
  return mail ? mail.slice(0, 2).toUpperCase() : '?'
})
// Set when logout failed: the server kept the session, so the editor is
// still logged in, and says so rather than pretending otherwise.
const logoutFailed = ref(false)
async function logOut() {
  logoutFailed.value = false
  try {
    // With OIDC the browser is on its way to the login page already.
    if (await auth.signOut()) return
  } catch {
    logoutFailed.value = true
    return
  }
  await navigateTo('/login')
}

// Content types are not nav items: they are a column in the Content list and a
// choice when creating a document.
const panelNav = [
  { to: '/dashboard', label: `⌂ ${t('Dashboard')}` },
  { to: '/content', label: t('Content') },
  { to: '/blocks', label: t('Block Library') },
  { to: '/assets', label: t('Assets') },
  { to: '/datasources', label: t('Datasources') },
]

// --- Help & inspiration (drop-up) + Sentry feedback ---
const showHelp = ref(false)
const { $reportProblem } = useNuxtApp()
async function reportProblem() {
  showHelp.value = false
  if ($reportProblem) await $reportProblem()
  else window.open('https://github.com/KlangHaus/resonans/issues/new', '_blank')
}
</script>

<template>
  <div class="sb-shell">
    <!-- Dark rail -->
    <aside class="sb-rail" :class="{ 'sb-rail--slim': inSpace }">
      <NuxtLink to="/" class="sb-rail__brand" title="resonans" @click="leave()">
        <span class="sb-rail__mark">R</span> <span class="sb-rail__label">resonans</span>
      </NuxtLink>

      <!-- Current-space chip: only in the slim rail, inside a space -->
      <div v-if="inSpace && !auth.enabled.value" class="sb-rail__spacechip" :title="active?.name">
        {{ (active?.name || '?').slice(0, 2).toUpperCase() }}
      </div>
      <div v-if="inSpace && !auth.enabled.value" class="sb-rail__sep" />

      <nav v-if="!auth.enabled.value" class="sb-rail__nav">
        <NuxtLink to="/" class="sb-rail__item" :class="{ on: route.path === '/' || inSpace }" :title="t('My Spaces')" @click="leave()">
          <span class="sb-rail__ic">▢</span> <span class="sb-rail__label">{{ t('My Spaces') }}</span>
        </NuxtLink>
        <NuxtLink to="/organization" class="sb-rail__item" :class="{ on: route.path === '/organization' }" :title="t('My organization')">
          <span class="sb-rail__ic">🏢</span> <span class="sb-rail__label">{{ t('My organization') }}</span>
        </NuxtLink>
      </nav>

      <div class="sb-rail__bottom">
        <!-- Help & inspiration: drop-up menu -->
        <div class="sb-help">
          <div v-if="showHelp" class="sb-help__menu">
            <div class="sb-help__title">{{ t('How can we help?') }}</div>
            <button type="button" class="sb-help__link" @click="reportProblem">{{ t('Report a problem') }}</button>
            <a class="sb-help__link" href="https://github.com/KlangHaus/resonans#readme" target="_blank" rel="noopener">{{ t('Documentation') }}</a>
            <a class="sb-help__link" href="https://github.com/KlangHaus/resonans/blob/main/CHANGELOG.md" target="_blank" rel="noopener">{{ t('Changelog') }}</a>
            <a class="sb-help__link" href="https://github.com/KlangHaus/resonans/issues" target="_blank" rel="noopener">{{ t('Community') }}</a>
          </div>
          <button type="button" class="sb-rail__item" :class="{ on: showHelp }" :title="t('Help & inspiration')" @click="showHelp = !showHelp">
            <span class="sb-rail__ic">◎</span> <span class="sb-rail__label">{{ t('Help & inspiration') }}</span>
          </button>
        </div>

        <!--
          The account row is the only place the Studio says WHO it thinks you
          are. With login enabled it shows the signed-in address and can sign
          out; in dev mode (no auth service configured) it keeps the old static
          label rather than claiming an identity nobody proved.
        -->
        <button
          v-if="auth.enabled.value"
          class="sb-rail__item sb-rail__account"
          :title="auth.user.value?.email || t('Not logged in')"
          type="button"
          @click="logOut"
        >
          <span class="sb-rail__avatar">{{ initials }}</span>
          <span class="sb-rail__label">{{ auth.user.value ? `${auth.user.value.email} · ${t('Log out')}` : t('Log in') }}</span>
        </button>
        <div v-else class="sb-rail__item sb-rail__account" :title="t('My account (development mode, no login)')">
          <span class="sb-rail__avatar">?</span> <span class="sb-rail__label">{{ t('My account') }}</span>
        </div>
        <!-- After the v-if/v-else pair, never between: a v-else pairs with the
             nearest v-if before it. -->
        <p v-if="logoutFailed" class="rz-field__hint" role="alert" style="padding: 0 12px">
          {{ t('Logout failed. You are still logged in.') }}
        </p>
      </div>
    </aside>

    <!-- Contextual doc-type panel (inside a space) -->
    <aside v-if="inSpace" class="sb-panel">
      <button v-if="!auth.enabled.value" type="button" class="sb-panel__back" @click="backToSpaces">← {{ t('Spaces') }}</button>
      <div class="sb-panel__space">
        <span class="sb-panel__spaceavatar">{{ (active?.name || '?').slice(0, 1) }}</span>
        <strong>{{ active?.name }}</strong>
      </div>
      <nav class="sb-panel__nav">
        <NuxtLink v-for="item in panelNav" :key="item.to" :to="item.to" class="sb-panel__item">
          {{ item.label }}
        </NuxtLink>
        <p v-if="error" class="rz-field__hint" style="padding: 0 12px">
          {{ t('Could not load the schema. Is the backend running?') }}
        </p>
      </nav>
      <div class="sb-panel__foot">
        <NuxtLink to="/settings" class="sb-panel__item">⚙ {{ t('Project settings') }}</NuxtLink>
      </div>
    </aside>

    <main class="sb-main">
      <NuxtPage />
    </main>

  </div>
</template>

<style scoped>
.sb-shell {
  display: flex;
  height: 100vh;
}
/* Rail */
.sb-rail {
  width: 232px;
  flex: none;
  background: var(--rz-rail-bg, #14161c); /* permanently dark: --rz-rail-* are theme-independent */
  color: var(--rz-rail-text-muted, #8b8f9c);
  display: flex;
  flex-direction: column;
  padding: 14px 10px;
  transition: width 0.15s ease;
}
/* Slim icon rail inside a space */
.sb-rail--slim {
  width: 60px;
  padding: 14px 8px;
  align-items: center;
}
.sb-rail--slim .sb-rail__label {
  display: none;
}
.sb-rail--slim .sb-rail__brand {
  padding: 6px 0 14px;
}
.sb-rail--slim .sb-rail__item {
  justify-content: center;
  padding: 9px 0;
  width: 44px;
}
.sb-rail__spacechip {
  width: 34px;
  height: 34px;
  border-radius: 9px;
  background: linear-gradient(135deg, #e94560, #c4a35a);
  color: #1a1a2e;
  display: grid;
  place-items: center;
  font-weight: 800;
  font-size: 12px;
  box-shadow: 0 0 0 2px var(--rz-rail-active-bg, #242835);
}
.sb-rail__sep {
  width: 28px;
  height: 1px;
  background: var(--rz-rail-border, #262a35);
  margin: 12px 0;
}
.sb-rail--slim .sb-help__menu {
  left: 52px;
  right: auto;
  bottom: 0;
  width: 230px;
}
.sb-rail__brand {
  display: flex;
  align-items: center;
  gap: 9px;
  font-weight: 800;
  font-size: 16px;
  color: var(--rz-rail-text, #e8e9ed);
  padding: 6px 10px 18px;
}
.sb-rail__mark {
  width: 26px;
  height: 26px;
  border-radius: 7px;
  background: linear-gradient(135deg, #e94560, #c4a35a);
  display: grid;
  place-items: center;
  color: #1a1a2e;
  font-weight: 800;
  font-size: 14px;
}
.sb-rail__nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.sb-rail__item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 10px;
  border-radius: 8px;
  font-size: 13.5px;
  color: var(--rz-rail-text-muted, #8b8f9c);
  background: none;
  border: 0;
  cursor: pointer;
  font-family: inherit;
  text-align: left;
  width: 100%;
}
.sb-rail__item:hover {
  background: var(--rz-rail-raised, #1b1e26);
  color: var(--rz-rail-text, #e8e9ed);
}
.sb-rail__item.on {
  background: var(--rz-rail-active-bg, #242835);
  color: var(--rz-rail-text, #e8e9ed);
  font-weight: 600;
}
.sb-rail__ic {
  width: 18px;
  text-align: center;
  opacity: 0.85;
}
.sb-rail__bottom {
  margin-top: auto;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.sb-rail__avatar {
  width: 24px;
  height: 24px;
  border-radius: 999px;
  background: linear-gradient(135deg, #e94560, #c4a35a);
  color: #1a1a2e;
  display: grid;
  place-items: center;
  font-size: 10px;
  font-weight: 800;
}
.sb-rail__account {
  cursor: default;
}
/* Help drop-up */
.sb-help {
  position: relative;
}
.sb-help__menu {
  position: absolute;
  bottom: calc(100% + 8px);
  left: 6px;
  right: 6px;
  background: var(--rz-raised, #fff);
  color: var(--rz-text, #1a1a2e);
  border-radius: 10px;
  box-shadow: 0 14px 40px rgba(0, 0, 0, 0.35);
  padding: 8px 0;
  z-index: 40;
}
.sb-help__title {
  font-size: 12px;
  font-weight: 700;
  padding: 8px 14px 10px;
  border-bottom: 1px solid var(--rz-border, #e6e2da);
  margin-bottom: 4px;
}
.sb-help__link {
  display: block;
  width: 100%;
  text-align: left;
  padding: 8px 14px;
  font-size: 13px;
  color: var(--rz-text, #1a1a2e);
  background: none;
  border: 0;
  cursor: pointer;
  font-family: inherit;
}
.sb-help__link:hover {
  background: var(--rz-surface, #f4f2ee);
  color: var(--rz-primary, #e94560);
}
/* Contextual panel */
.sb-panel {
  width: 208px;
  flex: none;
  background: var(--rz-bg-subtle, #f4f2ee);
  border-right: 1px solid var(--rz-border, #e6e2da);
  display: flex;
  flex-direction: column;
  padding: 12px 0;
  overflow: auto;
}
.sb-panel__back {
  background: none;
  border: 0;
  cursor: pointer;
  font-family: var(--rz-font-mono, ui-monospace, monospace);
  font-size: 11px;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--rz-text-muted, #6b6b78);
  text-align: left;
  padding: 2px 14px 10px;
}
.sb-panel__back:hover {
  color: var(--rz-text, #1a1a2e);
}
.sb-panel__space {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 4px 14px 12px;
  border-bottom: 1px solid var(--rz-border, #e6e2da);
  margin-bottom: 8px;
}
.sb-panel__spaceavatar {
  width: 26px;
  height: 26px;
  border-radius: 7px;
  background: linear-gradient(135deg, #e94560, #c4a35a);
  color: #1a1a2e;
  display: grid;
  place-items: center;
  font-weight: 800;
  font-size: 12px;
  text-transform: uppercase;
}
.sb-panel__nav {
  display: flex;
  flex-direction: column;
}
.sb-panel__item {
  padding: 8px 16px;
  text-transform: capitalize;
  color: var(--rz-text-muted, #6b6b78);
  font-size: 13.5px;
}
.sb-panel__item.router-link-active {
  color: var(--rz-text, #1a1a2e);
  background: var(--rz-surface, #fff);
  border-left: 2px solid var(--rz-primary, #e94560);
  font-weight: 600;
}
.sb-panel__foot {
  margin-top: auto;
  border-top: 1px solid var(--rz-border, #e6e2da);
  padding-top: 8px;
}
.sb-main {
  flex: 1;
  overflow: auto;
  padding: 22px 28px;
}
</style>
