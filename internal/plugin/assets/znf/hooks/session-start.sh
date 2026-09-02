#!/usr/bin/env bash
# znf SessionStart: emit the workflow digest UNLESS the user already carries
# local house-rules (sentinel in ~/.claude/CLAUDE.md) — stay silent then, to
# avoid double-loading discipline for the kit author.
set -euo pipefail
claude_md="$HOME/.claude/CLAUDE.md"
if [ -f "$claude_md" ] && grep -q 'znf-discipline-local' "$claude_md" 2>/dev/null; then
  exit 0
fi
cat "$CLAUDE_PLUGIN_ROOT/skills/using-superpowers/BOOTSTRAP.txt" 2>/dev/null || true
