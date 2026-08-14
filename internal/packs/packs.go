// Package packs embeds curated custom-rule packs shipped with the crivo binary.
//
// Packs are versioned with the binary: a pack bump is a crivo release. A remote
// registry is a future decision (see docs/custom-rules-design.md). Enable a pack
// from .qualitygate.yaml with:
//
//	include:
//	  - "pack:security-ts"
package packs

import (
	"embed"
	"fmt"
)

//go:embed *.yaml
var fs embed.FS

// Load returns the YAML content of the named embedded pack.
func Load(name string) ([]byte, error) {
	data, err := fs.ReadFile(name + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("pack %q not found (embedded packs: security-ts)", name)
	}
	return data, nil
}
