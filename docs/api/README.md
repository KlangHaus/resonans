# The API description

`openapi.yaml` describes every HTTP endpoint Resonans registers — 33 operations
across 26 paths, covering the editor API, the delivery API and the four
unauthenticated ones.

## It cannot go stale

A hand-written description next to a router is out of date within a month. This
repository proved it: the README once listed roughly fourteen routes where the
code had twenty-nine.

So the file is not the thing to trust. `internal/apidocs` is: a test that reads
the **syntax tree** of every package that registers a route and compares it to
this file, in both directions. Add a route without documenting it and `go test
./...` turns red, naming the route and the file and line it was registered on.
Document a route that does not exist and it turns red the other way. The outcome
always states its denominator — *"33 of 33 registered routes documented"* — so a
green cannot be confused with a green that measured nothing.

It reads the syntax tree rather than a shared table of routes on purpose. A table
that both the router and the test consulted would look like the production path
exactly where it is not one: a route registered straight onto the mux would
bypass the table and stay invisible. And it reads the tree rather than grepping,
because one real registration is built by concatenation
(`"GET " + media.AssetPath + "{key...}"`) and a line-bound pattern reading only
the first literal reports a method with an **empty path** — a phantom that passes
as one more documented endpoint.

A pattern the test cannot resolve is a **failure**, not a skip. A route it cannot
read is a route it cannot see, and a guard that quietly ignores what it does not
understand is worse than no guard.

## Reading it

Any OpenAPI viewer will do. Locally, without installing anything permanent:

```bash
npx @scalar/cli@latest document serve docs/api/openapi.yaml
```

**There is deliberately no HTML page in this repository that renders the spec
from a CDN.** An earlier attempt at exactly that was removed after a legal
finding: the page pulled its viewer from a third-party CDN, which made every
reader's browser contact that host without consent, and it exposed the internal
path list publicly on top of it. A hosted reading surface has to serve a
**static, build-time** artifact from our own origin — a live introspection
endpoint is not an option.
