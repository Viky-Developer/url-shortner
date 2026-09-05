#!/usr/bin/env bash
set -euo pipefail

ALLOWED_BRANCHES="main dev migrations"

branch="$(git branch --show-current)"

if [ -z "$branch" ]; then
  echo "error: unable to determine current branch" >&2
  exit 1
fi

for allowed in $ALLOWED_BRANCHES; do
  if [ "$branch" = "$allowed" ]; then
    exit 0
  fi
done

# feat|refactor|bug|fix|chore|hotfix / <issue_no> / <branch-name>
if ! [[ "$branch" =~ ^(feat|refactor|bug|fix|chore|hotfix)/[0-9]+/.+ ]]; then
  echo "error: invalid branch name '$branch'" >&2
  echo "" >&2
  echo "expected format: <type>/<issue_no>/<branch-name>" >&2
  echo "" >&2
  echo "  type prefixes:" >&2
  echo "    feat      - new features" >&2
  echo "    refactor  - refactoring existing code" >&2
  echo "    bug       - bug fixes" >&2
  echo "    fix       - immediate fixes merged to dev/main" >&2
  echo "    hotfix    - urgent production fixes" >&2
  echo "    chore     - maintenance tasks" >&2
  echo "" >&2
  echo "  examples:" >&2
  echo "    feat/42/user-login-flow" >&2
  echo "    refactor/15/auth-cleanup" >&2
  echo "    fix/30/redirect-loop" >&2
  echo "" >&2
  echo "create a properly named branch, e.g.:" >&2
  echo "  git checkout -b feat/42/user-login-flow" >&2
  echo "or use:" >&2
  echo "  make branch type=feat issue=42 name=user-login-flow" >&2
  exit 1
fi
