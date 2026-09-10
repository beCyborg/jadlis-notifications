#!/bin/sh
# sync-upstream.sh — pull a newer upstream release into this fork.
#
#   tools/sync-upstream.sh 1.42.0
#
# Successor to the owner's old apply-patches.sh, which rebuilt the patched
# binary straight into the plugin cache. Here the fork itself is the source of
# truth, so the steps are:
#
#   1. fetch the upstream tag v<X>
#   2. merge it into main (our patches and fork-owned files survive the 3-way merge)
#   3. re-apply the deterministic fork rewrites (module path, installer REPO)
#   4. rebuild the darwin/arm64 binary and check that it reports <X>
#   5. bump plugin.json / marketplace.json to <X>
#   6. print the commands to tag and publish the release
#
# Nothing is committed or pushed: review the diff, then commit yourself.
set -eu

FORK_MODULE="github.com/beCyborg/jadlis-notifications"
UPSTREAM_MODULE="github.com/777genius/claude-notifications"
FORK_REPO="beCyborg/jadlis-notifications"
UPSTREAM_REPO="777genius/claude-notifications-go"
BIN="claude-notifications-darwin-arm64"

VER="${1:-}"
[ -n "$VER" ] || { echo "usage: tools/sync-upstream.sh <version>   e.g. 1.42.0" >&2; exit 2; }
case "$VER" in v*) VER="${VER#v}" ;; esac

ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"

command -v go >/dev/null || { echo "ERROR: go not found" >&2; exit 1; }
[ -z "$(git status --porcelain)" ] || { echo "ERROR: working tree is dirty" >&2; exit 1; }

# --- 1. upstream tag ---
# In this fork the plain tag v<X> is the FORK's release (it must point at the
# merged commit, because bin/install.sh downloads assets from the v<version>
# release of this repository). Upstream tags are therefore fetched into a
# separate refs/tags/upstream/* namespace instead of clobbering ours.
git remote get-url upstream >/dev/null 2>&1 || \
    git remote add upstream "https://github.com/$UPSTREAM_REPO.git"
echo "==> fetching upstream tag v$VER"
git fetch --quiet --force upstream "refs/tags/v$VER:refs/tags/upstream/v$VER" || {
    echo "ERROR: upstream has no tag v$VER" >&2; exit 1; }

# --- 2. merge ---
echo "==> merging upstream/v$VER into $(git rev-parse --abbrev-ref HEAD)"
if ! git merge --no-commit --no-ff "upstream/v$VER"; then
    echo "" >&2
    echo "Merge conflicts. Resolve them, keeping the fork side for:" >&2
    echo "  .claude-plugin/  config/config.json  hooks/hooks.json  bin/stop-gate.sh" >&2
    echo "  bin/pending-bg-tasks.py  README*.md  NOTICE.md  tools/  .github/workflows/" >&2
    echo "If internal/hooks/hooks.go or internal/notifier/notifier.go lost the fork" >&2
    echo "changes, re-apply: git apply patches/session-title-subtitle.patch" >&2
    echo "Then re-run this script (it is idempotent once the tree is clean)." >&2
    exit 1
fi

# --- 3. deterministic fork rewrites ---
echo "==> re-applying fork rewrites"
git grep -lI "$UPSTREAM_MODULE" -- '*.go' '*.mod' 2>/dev/null | while IFS= read -r f; do
    sed -i '' "s|$UPSTREAM_MODULE\\([^-]\\)|$FORK_MODULE\\1|g; s|$UPSTREAM_MODULE\$|$FORK_MODULE|" "$f"
    echo "    module path: $f"
done
for f in bin/install.sh bin/bootstrap.sh bin/install_test.sh bin/install_e2e_test.sh setup.sh; do
    [ -f "$f" ] || continue
    if grep -q "$UPSTREAM_REPO" "$f"; then
        sed -i '' "s|$UPSTREAM_REPO|$FORK_REPO|g" "$f"
        echo "    installer repo: $f"
    fi
done

# The fork patches must be present in the Go sources after the merge.
grep -q "readClaudeSessionTitle" internal/notifier/notifier.go || {
    echo "ERROR: session-title patch is missing from internal/notifier/notifier.go." >&2
    echo "       Re-apply it: git apply patches/session-title-subtitle.patch" >&2
    exit 1; }
grep -q "drop the action-summary suffix" internal/hooks/hooks.go || {
    echo "ERROR: action-summary patch is missing from internal/hooks/hooks.go." >&2
    exit 1; }

# Nothing outside attribution text may still point at upstream.
LEFTOVER=$(git grep -lI "777genius" -- . \
    ':(exclude)LICENSE' ':(exclude)NOTICE.md' ':(exclude)CHANGELOG.md' \
    ':(exclude)README.md' ':(exclude)README.en.md' ':(exclude)docs/**' \
    ':(exclude)tools/sync-upstream.sh' ':(exclude)patches/**' 2>/dev/null || true)
if [ -n "$LEFTOVER" ]; then
    echo "WARNING: upstream references left outside attribution text:" >&2
    printf '%s\n' "$LEFTOVER" >&2
    echo "         Check them by hand before releasing." >&2
fi

# --- 4. rebuild ---
echo "==> building bin/$BIN"
go build -trimpath -ldflags="-s -w" -o "bin/$BIN" ./cmd/claude-notifications
chmod +x "bin/$BIN"
GOT=$("./bin/$BIN" version | grep -Eo '[0-9]+\.[0-9]+\.[0-9]+' | head -1)
[ "$GOT" = "$VER" ] || { echo "ERROR: binary reports $GOT, expected $VER" >&2; exit 1; }
echo "    ok: $BIN reports $GOT"
go vet ./... >/dev/null

# --- 5. manifests ---
echo "==> bumping manifests to $VER"
for f in .claude-plugin/plugin.json .claude-plugin/marketplace.json; do
    sed -i '' "s|\"version\": \"[0-9]*\.[0-9]*\.[0-9]*\"|\"version\": \"$VER\"|g" "$f"
    python3 -c "import json,sys;json.load(open(sys.argv[1]))" "$f"
done

# --- 6. next steps ---
cat <<EOF

Done. Review the diff, then:

  claude plugin validate . --strict
  git add -A && git commit -m "chore: sync upstream v$VER"
  git push origin main
  git tag -f v$VER && git push -f origin "v$VER"
  claude plugin tag . --push

  shasum -a 256 bin/$BIN | awk '{print \$1"  $BIN"}' > /tmp/checksums.txt
  gh release create v$VER bin/$BIN /tmp/checksums.txt -R $FORK_REPO \\
      --title "jadlis-notifications $VER" \\
      --notes "Fork of claude-notifications-go $VER with session-title subtitle, no action-summary suffix, stop gate, Russian defaults."
EOF
