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
