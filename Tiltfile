# Resonans — run the whole product locally with Tilt.
#
#   tilt up              the README's "Try it" stack: Postgres, migrations, the
#                        API, the editor and Dex as the login provider
#   tilt up -- --dev     the development stack instead (dev.Tiltfile): no login,
#                        the editor as a live-reloading dev server, the examples
#
# The default runs compose.yml + compose.try.yml with deploy/try/env, the same
# files and the same command the README gives an installer, so what you see
# here is what someone who clones the repository gets. Images are built from
# this checkout.
#
# Editor http://localhost:8080 (press log in: Dex approves everyone), API
# http://localhost:3000. Beside another stack that holds those ports:
#
#   STUDIO_PORT=3010 API_PORT=3015 tilt up --port 10351
#
# The editor's port is part of its login redirect, which Dex only accepts for
# the ports registered in deploy/try/dex.yaml: 8080 and 3010.
#
# Remove the stack and its data with:
#   docker compose --env-file deploy/try/env -f compose.yml -f compose.try.yml down -v

config.define_bool('dev')
cfg = config.parse()

if cfg.get('dev', False):
    include('dev.Tiltfile')
else:
    studio_port = os.getenv('STUDIO_PORT', '8080')
    api_port = os.getenv('API_PORT', '3000')
    if studio_port not in ['8080', '3010']:
        fail('STUDIO_PORT=' + studio_port + ': Dex accepts the login redirect on 8080 or 3010 only (deploy/try/dex.yaml).')
    editor_url = 'http://localhost:' + studio_port
    api_url = 'http://localhost:' + api_port
    # Compose reads these from the environment before deploy/try/env, so they
    # override it, and keep the URLs the stack builds in step with its ports.
    os.putenv('STUDIO_PORT', studio_port)
    os.putenv('API_PORT', api_port)
    os.putenv('EDITOR_URL', editor_url)
    os.putenv('PUBLIC_URL', api_url)

    docker_compose(
        ['compose.yml', 'compose.try.yml'],
        env_file='deploy/try/env',
        project_name='resonans',
    )

    dc_resource('postgres', new_name='resonans-postgres', labels=['resonans'])
    dc_resource('migrate', new_name='resonans-migrate', labels=['resonans'])
    dc_resource('dex', new_name='resonans-dex', labels=['resonans'],
                links=[link('http://dex.localhost:5556/dex/.well-known/openid-configuration', 'Dex discovery')])
    dc_resource('api', new_name='resonans-api', labels=['resonans'],
                links=[link(api_url + '/readyz', 'API'), link(api_url + '/docs/v1/', 'API reference')])
    dc_resource('studio', new_name='resonans-studio', labels=['resonans'],
                links=[link(editor_url, 'Resonans editor')])
