package parallel_reviews

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/kingrea/The-Lattice/internal/artifact"
	"github.com/kingrea/The-Lattice/internal/module"
	"github.com/kingrea/The-Lattice/internal/modules/runtime"
)

const (
	moduleID      = "parallel-reviews"
	moduleVersion = "1.0.0"
)

type reviewer struct {
	name        string
	artifactRef artifact.ArtifactRef
	personality string
}

var reviewerConfigs = []reviewer{
	{
		name:        "Pragmatist",
		artifactRef: artifact.ReviewPragmatistDoc,
		personality: `You are THE PRAGMATIST. Your role is to review plans with a focus on practical execution.
Ask yourself: Can this actually be built with the resources available? What are the real-world constraints?
Look for: Overly ambitious timelines, missing dependencies, resource assumptions, integration complexity.
Your tone: Direct, grounded, focused on "what will actually happen" not "what we hope happens."
Write your review to the specified file.`,
	},
	{
		name:        "Simplifier",
		artifactRef: artifact.ReviewSimplifierDoc,
		personality: `You are THE SIMPLIFIER. Your role is to find unnecessary complexity and propose simpler alternatives.
Ask yourself: Is this the simplest solution that could work? What can be removed or combined?
Look for: Over-engineering, premature abstraction, features that could be deferred, redundant components.
Your tone: Minimalist, questioning every addition, advocating for "less but better."
Write your review to the specified file.`,
	},
	{
		name:        "User Advocate",
		artifactRef: artifact.ReviewAdvocateDoc,
		personality: `You are THE USER ADVOCATE. Your role is to represent the end user's perspective.
Ask yourself: Will users actually want this? Is the experience being considered at every level?
Look for: Technical solutions looking for problems, missing user journeys, accessibility gaps, friction points.
Your tone: Empathetic, user-focused, always bringing it back to "but what does the user experience?"
Write your review to the specified file.`,
	},
	{
		name:        "Skeptic",
		artifactRef: artifact.ReviewSkepticDoc,
		personality: `You are THE SKEPTIC. Your role is to stress-test assumptions and find hidden risks.
Ask yourself: What could go wrong? What are we assuming that might not be true?
Look for: Unstated assumptions, single points of failure, security concerns, scalability issues, edge cases.
Your tone: Questioning, devil's advocate, not negative but rigorously probing.
Write your review to the specified file.`,
	},
}

// ParallelReviewsModule spawns four tmux windows (one per reviewer) and tracks
// the resulting artifacts.
type ParallelReviewsModule struct {
	*module.Base
	windowNames []string
}

// Register installs the module factory.
func Register(reg *module.Registry) {
	if reg == nil {
		return
	}
	reg.MustRegister(moduleID, func(module.Config) (module.Module, error) {
		return New(), nil
	})
}

// New configures module metadata and IO contracts.
func New() *ParallelReviewsModule {
	info := module.Info{
		ID:          moduleID,
		Name:        "Parallel Reviews",
		Description: "Runs the four reviewer personas against the plan.",
		Version:     moduleVersion,
		Concurrency: module.ConcurrencyProfile{Slots: 4},
	}
	base := module.NewBase(info)
	base.SetInputs(
		artifact.CommissionDoc,
		artifact.ArchitectureDoc,
		artifact.ConventionsDoc,
		artifact.ModulesDoc,
		artifact.ActionPlanDoc,
	)
	base.SetOutputs(
		artifact.ReviewPragmatistDoc,
		artifact.ReviewSimplifierDoc,
		artifact.ReviewAdvocateDoc,
		artifact.ReviewSkepticDoc,
	)
	return &ParallelReviewsModule{Base: &base}
}

// Run validates prerequisites and starts the reviewer sessions when needed.
func (m *ParallelReviewsModule) Run(ctx *module.ModuleContext) (module.Result, error) {
	if err := runtime.ValidateContext(moduleID, ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if missing, err := m.missingInput(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	} else if missing != "" {
		return module.Result{Status: module.StatusNeedsInput, Message: fmt.Sprintf("waiting for %s", missing)}, nil
	}
	if complete, err := m.IsComplete(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	} else if complete {
		return module.Result{Status: module.StatusNoOp, Message: "parallel reviews already complete"}, nil
	}
	if len(m.windowNames) > 0 {
		return module.Result{Status: module.StatusNeedsInput, Message: fmt.Sprintf("parallel reviews running in %d windows", len(m.windowNames))}, nil
	}
	planDir := ctx.Workflow.PlanDir()
	actionDir := ctx.Workflow.ActionDir()
	windows := make([]string, len(reviewerConfigs))
	for i, reviewer := range reviewerConfigs {
		window := fmt.Sprintf("review-%s-%d", strings.ToLower(reviewer.name), time.Now().Unix())
		if err := createTmuxWindow(window, ctx.Config.ProjectDir); err != nil {
			m.killWindows(windows[:i])
			return module.Result{Status: module.StatusFailed}, fmt.Errorf("parallel-reviews: create window for %s: %w", reviewer.name, err)
		}
		reviewPath := reviewer.artifactRef.Path(ctx.Workflow)
		prompt := fmt.Sprintf(
			"%s Read all planning documents from %s and action plan from %s. Write your review to %s. Be specific and actionable. Do not end until your review file is written.",
			reviewer.personality,
			planDir,
			actionDir,
			reviewPath,
		)
		if err := runOpenCode(window, prompt); err != nil {
			m.killWindows(windows[:i+1])
			return module.Result{Status: module.StatusFailed}, fmt.Errorf("parallel-reviews: launch %s: %w", reviewer.name, err)
		}
		windows[i] = window
		time.Sleep(500 * time.Millisecond)
	}
	m.windowNames = windows
	return module.Result{Status: module.StatusNeedsInput, Message: fmt.Sprintf("parallel reviews running in %d windows", len(windows))}, nil
}

// IsComplete checks whether all reviewer artifacts exist with metadata.
func (m *ParallelReviewsModule) IsComplete(ctx *module.ModuleContext) (bool, error) {
	if err := runtime.ValidateContext(moduleID, ctx); err != nil {
		return false, err
	}
	inputs := runtime.WithInputs(m.Inputs()...)
	for _, reviewer := range reviewerConfigs {
		ready, err := runtime.EnsureDocument(ctx, moduleID, moduleVersion, reviewer.artifactRef, inputs)
		if err != nil {
			return false, err
		}
		if !ready {
			return false, nil
		}
	}
	m.killWindows(m.windowNames)
	m.windowNames = nil
	return true, nil
}

func (m *ParallelReviewsModule) missingInput(ctx *module.ModuleContext) (string, error) {
	for _, ref := range m.Inputs() {
		result, err := ctx.Artifacts.Check(ref)
		if err != nil {
			return "", fmt.Errorf("parallel-reviews: check %s: %w", ref.ID, err)
		}
		if result.State != artifact.StateReady {
			return ref.Name, nil
		}
	}
	return "", nil
}

func (m *ParallelReviewsModule) killWindows(names []string) {
	for _, name := range names {
		if name == "" {
			continue
		}
		killTmuxWindow(name)
	}
}

func createTmuxWindow(name, dir string) error {
	args := []string{"new-window", "-n", name}
	if strings.TrimSpace(dir) != "" {
		args = append(args, "-c", dir)
	}
	cmd := exec.Command("tmux", args...)
	return cmd.Run()
}

func killTmuxWindow(name string) {
	if name == "" {
		return
	}
	_ = exec.Command("tmux", "kill-window", "-t", name).Run()
}

func runOpenCode(window, prompt string) error {
	escaped := strings.ReplaceAll(prompt, "\"", `\"`)
	escaped = strings.ReplaceAll(escaped, "\n", " ")
	cmd := exec.Command("tmux", "send-keys", "-t", window, fmt.Sprintf(`opencode --prompt "%s"`, escaped), "Enter")
	return cmd.Run()
}
