#!/usr/bin/env bash
# Exit 0 (true) if files matching given patterns have working-tree changes.
# Exit 1 (false) if nothing relevant changed.
#
# Usage:
#   hack/changed-paths.sh <pathspec>...
#
# Arguments are git pathspecs (directory prefixes or :(glob) patterns).
# Checks both uncommitted/unstaged changes and untracked files.
#
# Note: requires at least one commit (always true in kserve).
set -euo pipefail

changed=false

# Uncommitted changes (staged + unstaged)
if git diff --name-only HEAD -- "$@" | grep -q .; then changed=true; fi

# Untracked files matching the patterns
if [ "$changed" = false ] && \
   git ls-files --others --exclude-standard -- "$@" | grep -q .; then
  changed=true
fi

# Exit with the result: 'true' exits 0, 'false' exits 1 (terminates via set -e).
$changed
