package modules

import (
	"github.com/yourusername/lattice/internal/module"
	"github.com/yourusername/lattice/internal/modules/action_plan"
	"github.com/yourusername/lattice/internal/modules/anchor_docs"
	"github.com/yourusername/lattice/internal/modules/bead_creation"
	"github.com/yourusername/lattice/internal/modules/consolidation"
	"github.com/yourusername/lattice/internal/modules/hiring"
	"github.com/yourusername/lattice/internal/modules/orchestrator_selection"
	"github.com/yourusername/lattice/internal/modules/parallel_reviews"
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
	bead_creation.Register(reg)
	consolidation.Register(reg)
	orchestrator_selection.Register(reg)
	parallel_reviews.Register(reg)
	hiring.Register(reg)
	staff_incorporate.Register(reg)
	staff_review.Register(reg)
}
