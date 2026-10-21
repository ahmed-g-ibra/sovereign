#!/usr/bin/env bash
# Publish the site to GitHub Pages.
#
# The build happens here, not on GitHub, because the site renders a snapshot
# exported from the local database. A CI-side build would succeed and produce
# a correct, empty site.
#
#   ops/deploy-pages.sh            # unlisted (default)
#   INDEXABLE=true CONTACT=you@example.com ops/deploy-pages.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="/opt/homebrew/bin:/usr/local/bin:$PATH"

REPO_NAME="${REPO_NAME:-sovereign}"
OWNER="${OWNER:-ahmed-g-ibra}"
BRANCH="${BRANCH:-gh-pages}"
WORKTREE="$(mktemp -d)/site"

cd "$ROOT/api"
DATABASE_URL="${DATABASE_URL:-postgresql://sovereign:sovereign@localhost:55432/sovereign}" \
  uv run sovereign snapshot

cd "$ROOT/web"
BUILD_ENV=(
  "PUBLIC_BASE=/$REPO_NAME"
  "PUBLIC_SITE_URL=https://$OWNER.github.io"
)
[ "${INDEXABLE:-}" = "true" ] && BUILD_ENV+=("PUBLIC_INDEXABLE=true")
[ -n "${CONTACT:-}" ] && BUILD_ENV+=("PUBLIC_CONTACT_EMAIL=$CONTACT")
env "${BUILD_ENV[@]}" npm run build

# Pages runs Jekyll by default, and Jekyll skips directories beginning with an
# underscore -- which is every Astro asset. Without this the site loads with no
# styling and no JavaScript.
touch dist/.nojekyll

# _headers is a Cloudflare directive file. Pages ignores it, and shipping it
# would imply response headers that are not applied. On Pages the noindex meta
# tag is what does the work.
rm -f dist/_headers

# Always build the deploy on a throwaway orphan branch and force-push it.
#
# Reusing the existing branch is the trap here: -B would reset it to main and
# the next `git add -A` would publish the entire source tree. A fresh orphan
# time guarantees the branch contains the build output and nothing else.
TMP_BRANCH="deploy-$(date -u +%Y%m%d%H%M%S)-$$"

cd "$ROOT"
git worktree add --orphan -b "$TMP_BRANCH" "$WORKTREE" >/dev/null
cp -R web/dist/. "$WORKTREE/"

cd "$WORKTREE"
git add -A
git commit -q -m "Deploy site: data generated $(date -u +%Y-%m-%dT%H:%MZ)

Built from main with PUBLIC_BASE=/$REPO_NAME. Build output only; the
source lives on main."
git push -q -f origin "HEAD:$BRANCH"

cd "$ROOT"
git worktree remove --force "$WORKTREE"
git branch -D "$TMP_BRANCH" >/dev/null 2>&1 || true

echo
echo "deployed: https://$OWNER.github.io/$REPO_NAME/"
[ "${INDEXABLE:-}" = "true" ] && echo "WARNING: this build is indexable by search engines."
exit 0
