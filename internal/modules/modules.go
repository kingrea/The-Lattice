package modules

import (
	"github.com/yourusername/lattice/internal/module"
	"github.com/yourusername/lattice/internal/modules/action_plan"
	"github.com/yourusername/lattice/internal/modules/anchor_docs"
	"github.com/yourusername/lattice/internal/modules/staff_incorporate"
	"github.com/yourusername/lattice/internal/modules/staff_review"
)

// RegisterBuiltins installs all of the built-in module factories into the
// provided registry.
func RegisterBuiltins(reg *module.Registry) {
	if reg == nil {
		return
	}
	anchor_docs.Register(reg)
	action_plan.Register(reg)
	staff_incorporate.Register(reg)
	staff_review.Register(reg)
}
