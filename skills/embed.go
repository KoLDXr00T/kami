// Package skills holds the Claude Code skills shipped with Kami. The worker
// protocol is embedded so every agent step carries it.
package skills

import _ "embed"

//go:embed kami-worker/SKILL.md
var Worker string
