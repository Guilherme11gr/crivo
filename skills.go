package qualitygate

import "embed"

// SkillsFS contains the embedded Claude Code skills shipped with the binary.
// Files are rooted at .claude/skills/ (e.g. "ci/SKILL.md", "ci/scripts/qg-gate.sh").
//
//go:embed .claude/skills/ci/SKILL.md .claude/skills/ci/scripts/*.sh
var SkillsFS embed.FS
