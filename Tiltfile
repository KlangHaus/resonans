# Resonans CMS — local dev (Tilt-on-Compose, Beslutning P).
#
# Standalone:            cd resonans && tilt up
# From central Tiltfile: include('../resonans/Tiltfile')   # adjust relative path
#
# Dev-mode needs NO secrets (see docker-compose.tilt.yml): tenant via
# X-Dev-Tenant (editor) / X-Scope-OrgID (delivery), in-memory storage, boot
# auto-migrate. Server on http://localhost:3000 (healthz: /healthz).

docker_compose('docker-compose.tilt.yml')

# Tilt builds the Go image from the compose `build:` context and live-reloads on
# source changes. Group everything under a 'resonans' label.
dc_resource('postgres', labels=['resonans'])
dc_resource('resonans', labels=['resonans'])

# Manual button: seed the explainers demo course (all block types + quiz + page)
# for the dev tenant 11111111-1111-1111-1111-111111111111. Run once after up.
local_resource(
    'resonans-seed',
    cmd='docker compose -f docker-compose.tilt.yml --profile seed run --rm resonans-seed',
    labels=['resonans'],
    auto_init=False,
    trigger_mode=TRIGGER_MODE_MANUAL,
    resource_deps=['resonans'],
)

# --- Example: klanghaus.dk on Resonans (examples/klanghaus-site) ---
# The marketing site, landing pages and blog of klanghaus.dk, with every word and
# photo served by this Resonans. Press "klanghaus-site-seed" once after
# "resonans-seed" (the blog posts come from that one), then open
# http://localhost:3050.
local_resource(
    'klanghaus-site-seed',
    cmd='node seed/seed.mjs',
    dir='examples/klanghaus-site',
    env={'RESONANS_URL': 'http://localhost:3000', 'RESONANS_TENANT': '11111111-1111-1111-1111-111111111111'},
    labels=['examples'],
    auto_init=False,
    trigger_mode=TRIGGER_MODE_MANUAL,
    resource_deps=['resonans'],
)
# The editor (Resonans Studio) in dev mode — no login, tenant via X-Dev-Tenant —
# so live preview works out of the box: open a page and press "Visuel". The
# site's /preview route accepts frames only from this origin (studioOrigin).
local_resource(
    'resonans-studio',
    serve_cmd='pnpm install --frozen-lockfile && pnpm dev --port 3030',
    serve_dir='studio',
    serve_env={
        'RESONANS_DEV_API': 'http://localhost:3000',
        'NUXT_PUBLIC_DEV_TENANT': '11111111-1111-1111-1111-111111111111',
    },
    links=[link('http://localhost:3030', 'Resonans editor')],
    labels=['resonans'],
    resource_deps=['resonans'],
)
local_resource(
    'klanghaus-site',
    serve_cmd='pnpm install --frozen-lockfile && pnpm dev --port 3050',
    serve_dir='examples/klanghaus-site',
    serve_env={
        'NUXT_RESONANS_URL': 'http://localhost:3000',
        'NUXT_PUBLIC_RESONANS_PUBLIC_URL': 'http://localhost:3000',
        'NUXT_PUBLIC_STUDIO_ORIGIN': 'http://localhost:3030',
    },
    links=[link('http://localhost:3050', 'klanghaus.dk on Resonans')],
    labels=['examples'],
    resource_deps=['resonans'],
)
