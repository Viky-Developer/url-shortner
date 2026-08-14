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

if ! [[ "$branch" =~ ^(feat|refactor|bug|fix|chore|hotfix)/.+ ]]; then
  echo "error: invalid branch name '$branch'" >&2
  echo "" >&2
  echo "expected a context prefix before the branch name:" >&2
  echo "  feat/<name>     - new features" >&2
  echo "  refactor/<name> - refactoring existing code" >&2
  echo "  bug/<name>      - bug fixes" >&2
  echo "  fix/<name>      - immediate fixes merged to dev/main" >&2
  echo "  hotfix/<name>   - urgent production fixes" >&2
  echo "  chore/<name>    - maintenance tasks" >&2
  echo "" >&2
  echo "create a properly named branch, e.g.: git checkout -b feat/your-branch-name" >&2
  echo "or use: make branch type=feat name=your-branch-name" >&2
  exit 1
fi
