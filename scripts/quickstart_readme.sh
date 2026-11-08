#!/usr/bin/env bash
# Runs the README's "Try it" commands as they are written, in a fresh clone, the way
# a stranger would. The workflow used to carry its own copy of the compose command,
# so the README could change and CI would go on proving a command nobody was told
# to run.
#
#   quickstart_readme.sh README SOURCE_REPO WORKDIR
#
# The block must start with the README's `git clone` of the public repository and a
# `cd` into it. The clone is made from SOURCE_REPO instead (the checkout under test),
# with --no-local, so only committed files reach it: nothing ignored, untracked or
# built locally. Every other line runs as written, one at a time, from WORKDIR.
#
# Exit codes: 2 the block is missing or has changed shape, 3 a README step failed.
set -euo pipefail

readme="${1:?usage: quickstart_readme.sh README SOURCE_REPO WORKDIR}"
source_repo="${2:?usage: quickstart_readme.sh README SOURCE_REPO WORKDIR}"
workdir="${3:?usage: quickstart_readme.sh README SOURCE_REPO WORKDIR}"

clone_line='git clone https://github.com/KlangHaus/resonans.git'
cd_line='cd resonans'

# The first ```bash block under "### Try it", up to the next heading.
mapfile -t steps < <(awk '
  /^### Try it$/ { section = 1; next }
  section && /^#/ { exit }
  section && !inblock && /^```bash$/ { inblock = 1; next }
  inblock && /^```$/ { exit }
  inblock && NF { print }
' "$readme")

refuse() {
  echo "::error::the README's Try it block $1" >&2
  printf '  %s\n' "${steps[@]}" >&2
  exit 2
}

[ "${#steps[@]}" -ge 1 ] || refuse "was not found"
[ "${steps[0]}" = "$clone_line" ] || refuse "no longer starts with: $clone_line"
[ "${steps[1]:-}" = "$cd_line" ] || refuse "no longer continues with: $cd_line"
[ "${#steps[@]}" -ge 3 ] || refuse "has no command after the clone"

mkdir -p "$workdir"
cd "$workdir"

echo "step 1: ${steps[0]}"
echo "        (cloned from $source_repo, so the commit under test is what runs)"
git clone --no-local --quiet "$source_repo" resonans
echo "        $(git -C resonans rev-parse HEAD)"

for i in "${!steps[@]}"; do
  [ "$i" -ge 1 ] || continue
  step="${steps[$i]}"
  echo "step $((i + 1)): $step"
  rc=0
  eval "$step" || rc=$?
  if [ "$rc" -ne 0 ]; then
    echo "::error::README step $((i + 1)) failed (exit $rc): $step" >&2
    exit 3
  fi
done

echo "README Try it: all $((${#steps[@]})) steps ran"
