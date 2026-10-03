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

## It also has to be valid, which is a different question

The drift cell compares **route lists**. It says nothing about whether the document
is valid OpenAPI — and for a while it was not. Running the viewer this README names
reported **20 errors and exited 1**: every templated path (`{key}`, `{id}`, `{type}`,
`{slug}`, `{version}`) was declared in the path and never defined as a path
parameter. So the only documented way to read this API failed, while the guard that
covers the spec stayed green the whole time, because a route list and a parameter
definition are different questions.

`TestSpecIsValidOpenAPI` closes that. It validates the document offline with a
pinned Go library rather than shelling out to the JS viewer: an unpinned fetch at
test time would make a validator release read as our spec breaking. It asserts its
own denominator too — a document that parsed to zero paths would otherwise validate
perfectly.

The parameter types are read from the handlers, not guessed: `id` is parsed with
`uuid.Parse`, `version` with `strconv.Atoi`, and `type` is looked up in the schema
registry, so an unknown type is a 404 rather than a 400. `key` is the one that does
not fit the format — it is registered as a wildcard segment and may contain `/`,
which OpenAPI's simple style cannot model, so the parameter says so instead of
pretending otherwise.

What this does **not** measure, so the green does not overclaim: validity is not
accuracy. A response schema can be valid OpenAPI and still describe a shape the
handler never returns. Routes have a cell for that; schemas do not yet.

## Reading it

Any OpenAPI viewer will do. Locally, without installing anything permanent:

```bash
npx @scalar/cli@latest document serve docs/api/openapi.yaml
```

Measured after the parameter fix: `document validate` reports *"Matches the OpenAPI
specification (OpenAPI 3.1)"* and exits 0.

**There is deliberately no HTML page in this repository that renders the spec
from a CDN.** An earlier attempt at exactly that was removed after a legal
finding: the page pulled its viewer from a third-party CDN, which made every
reader's browser contact that host without consent, and it exposed the internal
path list publicly on top of it. A hosted reading surface has to serve a
**static, build-time** artifact from our own origin — a live introspection
endpoint is not an option.
