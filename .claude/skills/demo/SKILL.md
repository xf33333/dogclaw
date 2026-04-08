---
name: demo-skill
description: A demo skill for verification
arguments:
  - user
---
Hello ${user}! This is a demo skill in goclaude.
Dir: ${CLAUDE_SKILL_DIR}
Session: ${CLAUDE_SESSION_ID}
