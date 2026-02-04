package modules

import (
	"github.com/yourusername/lattice/internal/module"
	"github.com/yourusername/lattice/internal/modules/anchor_docs"
)

// RegisterBuiltins installs all of the built-in module factories into the
// provided registry.
func RegisterBuiltins(reg *module.Registry) {
	if reg == nil {
		return
	}
	anchor_docs.Register(reg)
}
