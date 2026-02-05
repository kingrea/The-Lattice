package modules

import (
	"github.com/kingrea/The-Lattice/internal/module"
	"github.com/kingrea/The-Lattice/internal/modules/action_plan"
	"github.com/kingrea/The-Lattice/internal/modules/anchor_docs"
	"github.com/kingrea/The-Lattice/internal/modules/bead_creation"
	"github.com/kingrea/The-Lattice/internal/modules/consolidation"
	"github.com/kingrea/The-Lattice/internal/modules/hiring"
	"github.com/kingrea/The-Lattice/internal/modules/orchestrator_selection"
	"github.com/kingrea/The-Lattice/internal/modules/parallel_reviews"
	"github.com/kingrea/The-Lattice/internal/modules/refinement"
	"github.com/kingrea/The-Lattice/internal/modules/release"
	"github.com/kingrea/The-Lattice/internal/modules/solo_work"
	"github.com/kingrea/The-Lattice/internal/modules/staff_incorporate"
	"github.com/kingrea/The-Lattice/internal/modules/staff_review"
	"github.com/kingrea/The-Lattice/internal/modules/work_process"
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
	refinement.Register(reg)
	release.Register(reg)
	solo_work.Register(reg)
	hiring.Register(reg)
	staff_incorporate.Register(reg)
	staff_review.Register(reg)
	work_process.Register(reg)
}
