// internal/modes/planning/planning.go
//
// Planning mode handles the initial planning session for a commission.
// It runs in multiple phases:
// 1. Run lattice-planning skill to create anchor docs
// 2. Create action plan (MODULES.md, PLAN.md)
// 3. Staff Engineer review
// 4. User decision: proceed or keep chatting
// 5. Parallel reviews by 4 personalities (Pragmatist, Simplifier, User Advocate, Skeptic)
// 6. Orchestrator consolidates feedback and applies changes
// 7. Create beads from modules and plan for work tracking
//
// Output: .lattice/plan/ and .lattice/action/

package planning

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/yourusername/lattice/internal/modes"
	"github.com/yourusername/lattice/internal/skills"
	"github.com/yourusername/lattice/internal/workflow"
)

// planningPhase tracks which sub-phase we're in
type planningPhase int

const (
	phaseInit               planningPhase = iota
	phaseAnchorDocs                       // Running lattice-planning skill
	phaseActionPlan                       // Creating MODULES.md and PLAN.md
	phaseStaffReview                      // Staff Engineer reviews the plan
	phaseStaffIncorporation               // Staff feedback is applied to the plan
	phaseUserDecision                     // User decides: proceed or keep chatting
	phasePlanChat                         // Collaborative chat cycles on the plan
	phaseParallelReviews                  // 4 parallel reviewers
	phaseConsolidation                    // Orchestrator applies feedback
	phaseBeadCreation                     // Create beads from modules and plan
	phaseComplete
)

// Reviewer personalities
type reviewer struct {
	name        string
	filename    string
	personality string
}

var reviewers = []reviewer{
	{
		name:     "Pragmatist",
		filename: workflow.FileReviewPragmatist,
		personality: `You are THE PRAGMATIST. Your role is to review plans with a focus on practical execution.
Ask yourself: Can this actually be built with the resources available? What are the real-world constraints?
Look for: Overly ambitious timelines, missing dependencies, resource assumptions, integration complexity.
Your tone: Direct, grounded, focused on "what will actually happen" not "what we hope happens."
Write your review to the specified file.`,
	},
	{
		name:     "Simplifier",
		filename: workflow.FileReviewSimplifier,
		personality: `You are THE SIMPLIFIER. Your role is to find unnecessary complexity and propose simpler alternatives.
Ask yourself: Is this the simplest solution that could work? What can be removed or combined?
Look for: Over-engineering, premature abstraction, features that could be deferred, redundant components.
Your tone: Minimalist, questioning every addition, advocating for "less but better."
Write your review to the specified file.`,
	},
	{
		name:     "User Advocate",
		filename: workflow.FileReviewAdvocate,
		personality: `You are THE USER ADVOCATE. Your role is to represent the end user's perspective.
Ask yourself: Will users actually want this? Is the experience being considered at every level?
Look for: Technical solutions looking for problems, missing user journeys, accessibility gaps, friction points.
Your tone: Empathetic, user-focused, always bringing it back to "but what does the user experience?"
Write your review to the specified file.`,
	},
	{
		name:     "Skeptic",
		filename: workflow.FileReviewSkeptic,
		personality: `You are THE SKEPTIC. Your role is to stress-test assumptions and find hidden risks.
Ask yourself: What could go wrong? What are we assuming that might not be true?
Look for: Unstated assumptions, single points of failure, security concerns, scalability issues, edge cases.
Your tone: Questioning, devil's advocate, not negative but rigorously probing.
Write your review to the specified file.`,
	},
}

// Mode handles the planning session phase
type Mode struct {
	modes.BaseMode
	width  int
	height int

	// Current sub-phase
	phase planningPhase

	// Window names for opencode sessions
	windowName  string
	windowNames []string // For parallel reviewers

	// User decision state
	cursorPos int // 0 = proceed, 1 = keep chatting
}

// New creates a new Planning mode
func New() *Mode {
	return &Mode{
		BaseMode: modes.NewBaseMode("Planning Session", workflow.PhasePlanning),
		phase:    phaseInit,
	}
}

// Init initializes the planning mode
func (m *Mode) Init(ctx *modes.ModeContext) tea.Cmd {
	m.SetContext(ctx)

	wf := ctx.Workflow

	// Ensure directories exist
	if err := wf.Initialize(); err != nil {
		m.SetStatusMsg(fmt.Sprintf("Error initializing workflow: %v", err))
		return func() tea.Msg {
			return modes.ModeErrorMsg{Error: err}
		}
	}

	// Check if planning is fully complete (beads created)
	if wf.PlanningComplete() {
		m.SetComplete(true)
		m.SetStatusMsg("Planning complete, advancing to orchestrator selection")
		return func() tea.Msg {
			return modes.ModeCompleteMsg{NextPhase: workflow.PhaseOrchestratorSelection}
		}
	}

	// Check if reviews are applied but beads not yet created
	if fileExists(wf.ReviewsAppliedPath()) && !fileExists(wf.BeadsCreatedPath()) {
		m.phase = phaseBeadCreation
		m.SetStatusMsg("Consolidation complete, creating beads for tracking...")
		return m.startBeadCreation()
	}

	// Check if all reviews are done but not yet consolidated
	if wf.AllReviewsComplete() && !fileExists(wf.ReviewsAppliedPath()) {
		m.phase = phaseConsolidation
		m.SetStatusMsg("Reviews complete, consolidating feedback...")
		return m.startConsolidation()
	}

	// Check if staff review is complete but we're still before parallel reviewers
	if fileExists(wf.StaffReviewPath()) && !wf.AllReviewsComplete() {
		if !fileExists(wf.StaffFeedbackAppliedPath()) {
			m.phase = phaseStaffIncorporation
			m.SetStatusMsg("Incorporating Staff Engineer feedback into the plan...")
			return m.startStaffFeedbackIncorporation()
		}

		if fileExists(wf.PlanChatReadyPath()) {
			removeFileIfExists(wf.PlanChatReadyPath())
			removeFileIfExists(wf.PlanChatActivePath())
			m.phase = phaseUserDecision
			m.SetStatusMsg("Planning chat wrapped up. Review the updated plan to proceed.")
			return nil
		}

		if fileExists(wf.PlanChatActivePath()) {
			m.phase = phasePlanChat
			m.SetStatusMsg("Continuing planning chat with the Staff Engineer...")
			return m.startPlanChatSession()
		}

		// Check if any reviews exist (meaning we started parallel reviews)
		if fileExists(wf.ReviewPragmatistPath()) || fileExists(wf.ReviewSimplifierPath()) ||
			fileExists(wf.ReviewAdvocatePath()) || fileExists(wf.ReviewSkepticPath()) {
			m.phase = phaseParallelReviews
			m.SetStatusMsg("Waiting for parallel reviews to complete...")
			return m.pollForCompletion()
		}

		// Staff feedback has been applied—ask the user to review MODULES/PLAN
		m.phase = phaseUserDecision
		m.SetStatusMsg("Staff feedback applied. Review MODULES.md and PLAN.md to decide next steps.")
		return nil
	}

	// Check if action plan exists but staff review doesn't
	if fileExists(wf.ModulesPath()) && fileExists(wf.ActionPlanPath()) && !fileExists(wf.StaffReviewPath()) {
		m.phase = phaseStaffReview
		m.SetStatusMsg("Action plan complete, starting Staff Engineer review...")
		return m.startStaffReview()
	}

	// Check if anchor docs are complete - skip to action plan phase
	if wf.AnchorDocsComplete() && (!fileExists(wf.ModulesPath()) || !fileExists(wf.ActionPlanPath())) {
		m.phase = phaseActionPlan
		m.SetStatusMsg("Anchor docs complete, creating action plan...")
		return m.startActionPlanSession()
	}

	// Start from the beginning
	m.phase = phaseAnchorDocs
	m.SetStatusMsg("Starting lattice-planning session...")
	return m.startAnchorDocsSession()
}

// Update handles messages for the planning mode
func (m *Mode) Update(msg tea.Msg) (modes.Mode, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Handle user decision phase differently
		if m.phase == phaseUserDecision {
			switch msg.String() {
			case "up", "k":
				if m.cursorPos > 0 {
					m.cursorPos--
				}
				return m, nil
			case "down", "j":
				if m.cursorPos < 1 {
					m.cursorPos++
				}
				return m, nil
			case "enter":
				if m.cursorPos == 0 {
					m.clearPlanChatMarkers()
					// Proceed with parallel reviews
					m.phase = phaseParallelReviews
					m.SetStatusMsg("Starting parallel reviews...")
					return m, m.startParallelReviews()
				} else {
					// Keep chatting about the plan in a dedicated session
					m.phase = phasePlanChat
					m.SetStatusMsg("Opening an OpenCode session to keep refining the plan...")
					return m, m.startPlanChatSession()
				}
			}
		}

		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.killAllWindows()
			return m, func() tea.Msg {
				return modes.ModeErrorMsg{Error: fmt.Errorf("planning cancelled")}
			}
		}

	case anchorDocsCompleteMsg:
		m.killWindow()
		m.phase = phaseActionPlan
		m.SetStatusMsg("Anchor docs complete! Creating action plan...")
		return m, m.startActionPlanSession()

	case actionPlanCompleteMsg:
		m.killWindow()
		m.phase = phaseStaffReview
		m.SetStatusMsg("Action plan complete! Starting Staff Engineer review...")
		return m, m.startStaffReview()

	case staffReviewCompleteMsg:
		m.killWindow()
		m.phase = phaseStaffIncorporation
		m.SetStatusMsg("Staff review complete. Applying feedback to the plan...")
		return m, m.startStaffFeedbackIncorporation()

	case staffFeedbackAppliedMsg:
		m.killWindow()
		m.phase = phaseUserDecision
		m.SetStatusMsg("Staff feedback applied. Review the updated plan and decide.")
		return m, nil

	case parallelReviewsCompleteMsg:
		m.killAllWindows()
		m.phase = phaseConsolidation
		m.SetStatusMsg("All reviews complete! Consolidating feedback...")
		return m, m.startConsolidation()

	case planChatCompleteMsg:
		m.killWindow()
		m.clearPlanChatMarkers()
		m.phase = phaseUserDecision
		m.SetStatusMsg("Planning chat complete. Review the updated plan to decide.")
		return m, nil

	case consolidationCompleteMsg:
		m.killWindow()
		m.phase = phaseBeadCreation
		m.SetStatusMsg("Consolidation complete! Creating beads for tracking...")
		return m, m.startBeadCreation()

	case beadsCreatedMsg:
		m.killWindow()
		m.SetComplete(true)
		m.SetStatusMsg("Planning complete! Beads created for work tracking.")
		return m, func() tea.Msg {
			return modes.ModeCompleteMsg{NextPhase: workflow.PhaseOrchestratorSelection}
		}

	case pollTickMsg:
		return m, m.pollForCompletion()

	case modes.ModeErrorMsg:
		m.SetStatusMsg(fmt.Sprintf("Error: %v", msg.Error))
		return m, nil
	}

	return m, nil
}

// View renders the planning mode
func (m *Mode) View() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFD93D")).
		Padding(1)

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		MarginTop(1)

	phaseStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00BFFF"))

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00FF00")).
		Bold(true)

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#AAAAAA"))

	// User decision view
	if m.phase == phaseUserDecision {
		content := titleStyle.Render(`
  📋 PLANNING SESSION - Decision Point

  The Staff Engineer's feedback has already been merged into the plan.
  Review the updated MODULES.md and PLAN.md in .lattice/action/

  What would you like to do?
`)
		var options string
		if m.cursorPos == 0 {
			options = selectedStyle.Render("  > Good to proceed with plan\n")
			options += normalStyle.Render("    Keep chatting about the plan")
		} else {
			options += normalStyle.Render("    Good to proceed with plan\n")
			options += selectedStyle.Render("  > Keep chatting about the plan")
		}

		hint := statusStyle.Render("\n\n  Use arrow keys to select, Enter to confirm")
		return fmt.Sprintf("%s\n%s%s\n%s", content, options, hint, statusStyle.Render(m.StatusMsg()))
	}

	var phaseText string
	switch m.phase {
	case phaseAnchorDocs:
		phaseText = "Phase 1/7: Creating anchor documents (COMMISSION, ARCHITECTURE, CONVENTIONS)"
	case phaseActionPlan:
		phaseText = "Phase 2/7: Creating action plan (MODULES, PLAN)"
	case phaseStaffReview:
		phaseText = "Phase 3/7: Staff Engineer reviewing the plan"
	case phaseStaffIncorporation:
		phaseText = "Phase 3/7: Applying Staff Engineer feedback to MODULES/PLAN"
	case phasePlanChat:
		phaseText = "Phase 4/7: Collaborative planning chat before proceeding"
	case phaseParallelReviews:
		phaseText = "Phase 5/7: Parallel reviews by 4 personalities"
	case phaseConsolidation:
		phaseText = "Phase 6/7: Consolidating feedback and applying changes"
	case phaseBeadCreation:
		phaseText = "Phase 7/7: Creating beads for work tracking"
	case phaseComplete:
		phaseText = "Planning complete!"
	default:
		phaseText = "Initializing..."
	}

	content := titleStyle.Render(`
  📋 PLANNING SESSION

  The planning session will:
  1. Create anchor docs (COMMISSION, ARCHITECTURE, CONVENTIONS)
  2. Create the action plan (MODULES, PLAN)
  3. Run a Staff Engineer review and apply that feedback to the plan
  4. Let you review the updated plan (keep chatting if needed)
  5. Run four reviewers in parallel:
     - Pragmatist, Simplifier, User Advocate, Skeptic
  6. Orchestrator consolidates and applies feedback
  7. Create beads from modules/plan for work tracking

  OpenCode sessions are running in tmux windows.

  Press ESC to cancel.
`)

	phaseInfo := phaseStyle.Render(fmt.Sprintf("\n  Current: %s\n", phaseText))

	return fmt.Sprintf("%s%s\n%s", content, phaseInfo, statusStyle.Render(m.StatusMsg()))
}

// Message types
type anchorDocsCompleteMsg struct{}
type actionPlanCompleteMsg struct{}
type staffReviewCompleteMsg struct{}
type staffFeedbackAppliedMsg struct{}
type parallelReviewsCompleteMsg struct{}
type consolidationCompleteMsg struct{}
type beadsCreatedMsg struct{}
type planChatCompleteMsg struct{}
type pollTickMsg struct{}

// startAnchorDocsSession spawns an opencode session with the lattice-planning skill
func (m *Mode) startAnchorDocsSession() tea.Cmd {
	return func() tea.Msg {
		ctx := m.Context()

		m.windowName = fmt.Sprintf("lattice-planning-%d", time.Now().Unix())
		if err := createTmuxWindow(m.windowName); err != nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("failed to create tmux window: %w", err)}
		}

		skillPath, err := skills.Ensure(ctx.Config.SkillsDir(), skills.LatticePlanning)
		if err != nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("failed to prepare planning skill: %w", err)}
		}
		planDir := ctx.Workflow.PlanDir()

		prompt := fmt.Sprintf(
			"Load and execute the skill at %s. "+
				"Create the three anchor documents (COMMISSION.md, ARCHITECTURE.md, CONVENTIONS.md) in %s. "+
				"Work with the user to gather requirements and make decisions. "+
				"Write each file as soon as that phase completes. "+
				"Do not end until all three files exist.",
			skillPath, planDir,
		)

		if err := runOpenCode(prompt, m.windowName); err != nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("failed to start opencode: %w", err)}
		}

		return pollTickMsg{}
	}
}

// startActionPlanSession spawns a session for MODULES.md and PLAN.md
func (m *Mode) startActionPlanSession() tea.Cmd {
	return func() tea.Msg {
		ctx := m.Context()

		m.killWindow()
		m.windowName = fmt.Sprintf("lattice-action-%d", time.Now().Unix())
		if err := createTmuxWindow(m.windowName); err != nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("failed to create tmux window: %w", err)}
		}

		planDir := ctx.Workflow.PlanDir()
		actionDir := ctx.Workflow.ActionDir()

		prompt := fmt.Sprintf(
			"You are creating an action plan based on completed planning documents. "+
				"Read the anchor documents from %s (COMMISSION.md, ARCHITECTURE.md, CONVENTIONS.md). "+
				"Based on these documents, create two files in %s: "+
				"1. MODULES.md - Break down the work into top-level parallelizable modules. "+
				"Each module should be independent enough to be worked on by a separate agent. "+
				"Include clear boundaries and interfaces between modules. "+
				"2. PLAN.md - Create an implementation plan that sequences the work. "+
				"Reference the modules and show dependencies. "+
				"Write both files. Do not end until both exist in %s.",
			planDir, actionDir, actionDir,
		)

		if err := runOpenCode(prompt, m.windowName); err != nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("failed to start opencode: %w", err)}
		}

		return pollTickMsg{}
	}
}

// startStaffReview spawns a Staff Engineer to review the plan
func (m *Mode) startStaffReview() tea.Cmd {
	return func() tea.Msg {
		ctx := m.Context()

		m.killWindow()
		m.windowName = fmt.Sprintf("staff-engineer-%d", time.Now().Unix())
		if err := createTmuxWindow(m.windowName); err != nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("failed to create tmux window: %w", err)}
		}

		planDir := ctx.Workflow.PlanDir()
		actionDir := ctx.Workflow.ActionDir()
		reviewPath := ctx.Workflow.StaffReviewPath()

		prompt := fmt.Sprintf(
			"You are a STAFF ENGINEER conducting a thorough review. "+
				"Read the planning documents from %s (COMMISSION.md, ARCHITECTURE.md, CONVENTIONS.md) "+
				"and the action plan from %s (MODULES.md, PLAN.md). "+
				"Review them as a staff engineer would: "+
				"- Are the modules well-defined and truly parallelizable? "+
				"- Is the plan realistic and complete? "+
				"- Are there gaps, risks, or unclear boundaries? "+
				"- What advice would you give before implementation begins? "+
				"Write your review to %s. "+
				"Be thorough but constructive. This review will inform the final plan. "+
				"Do not end until your review is written.",
			planDir, actionDir, reviewPath,
		)

		if err := runOpenCode(prompt, m.windowName); err != nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("failed to start opencode: %w", err)}
		}

		return pollTickMsg{}
	}
}

// startStaffFeedbackIncorporation applies the Staff Engineer review to the plan
func (m *Mode) startStaffFeedbackIncorporation() tea.Cmd {
	return func() tea.Msg {
		ctx := m.Context()

		m.killWindow()
		m.windowName = fmt.Sprintf("staff-incorporation-%d", time.Now().Unix())
		if err := createTmuxWindow(m.windowName); err != nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("failed to create tmux window: %w", err)}
		}

		planDir := ctx.Workflow.PlanDir()
		actionDir := ctx.Workflow.ActionDir()
		reviewPath := ctx.Workflow.StaffReviewPath()
		markerPath := ctx.Workflow.StaffFeedbackAppliedPath()

		prompt := fmt.Sprintf(
			"You already wrote the Staff Engineer review at %s. "+
				"Before the user sees anything, apply that feedback directly to the plan. "+
				"Read the planning docs in %s (COMMISSION.md, ARCHITECTURE.md, CONVENTIONS.md) and the current action plan in %s (MODULES.md, PLAN.md). "+
				"Update MODULES.md and PLAN.md so the guidance from your review is fully incorporated and clearly explained. "+
				"Add a short section near the top of PLAN.md summarizing the adjustments made. "+
				"When the updates are complete, create the marker file %s to signal that the user can now review the improved plan. "+
				"Do not ask the user to read the review file—deliver the updated plan instead.",
			reviewPath, planDir, actionDir, markerPath,
		)

		if err := runOpenCode(prompt, m.windowName); err != nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("failed to start opencode: %w", err)}
		}

		return pollTickMsg{}
	}
}

// startPlanChatSession opens a collaborative planning session with context
func (m *Mode) startPlanChatSession() tea.Cmd {
	return func() tea.Msg {
		ctx := m.Context()
		wf := ctx.Workflow

		readyPath := wf.PlanChatReadyPath()
		activePath := wf.PlanChatActivePath()
		removeFileIfExists(readyPath)
		removeFileIfExists(activePath)

		if err := os.WriteFile(activePath, []byte{}, 0644); err != nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("failed to write plan chat marker: %w", err)}
		}

		m.killWindow()
		m.windowName = fmt.Sprintf("plan-chat-%d", time.Now().Unix())
		if err := createTmuxWindow(m.windowName); err != nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("failed to create tmux window: %w", err)}
		}

		planDir := wf.PlanDir()
		actionDir := wf.ActionDir()
		reviewPath := wf.StaffReviewPath()

		prompt := fmt.Sprintf(
			"You are a STAFF ENGINEER facilitating an ongoing planning chat. "+
				"Your previous review (at %s) has already been incorporated into the plan. "+
				"Provide the user with a concise recap of what changed, then keep iterating on the plan together. "+
				"Reference the planning docs in %s and the action plan in %s when making updates. "+
				"Apply any new ideas directly to MODULES.md and PLAN.md. "+
				"Stay in this session until the user explicitly says they're ready to proceed. "+
				"Once they confirm, create the marker file %s so the TUI can return to the decision screen.",
			reviewPath, planDir, actionDir, readyPath,
		)

		if err := runOpenCode(prompt, m.windowName); err != nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("failed to start plan chat opencode: %w", err)}
		}

		return pollTickMsg{}
	}
}

// startParallelReviews spawns 4 parallel reviewers
func (m *Mode) startParallelReviews() tea.Cmd {
	return func() tea.Msg {
		ctx := m.Context()

		m.killWindow()
		m.windowNames = make([]string, len(reviewers))

		planDir := ctx.Workflow.PlanDir()
		actionDir := ctx.Workflow.ActionDir()

		for i, r := range reviewers {
			windowName := fmt.Sprintf("reviewer-%s-%d", strings.ToLower(r.name), time.Now().Unix())
			m.windowNames[i] = windowName

			if err := createTmuxWindow(windowName); err != nil {
				return modes.ModeErrorMsg{Error: fmt.Errorf("failed to create window for %s: %w", r.name, err)}
			}

			reviewPath := filepath.Join(actionDir, r.filename)
			prompt := fmt.Sprintf(
				"%s "+
					"Read all planning documents from %s and action plan from %s. "+
					"Write your review to %s. "+
					"Be specific and actionable. Do not end until your review file is written.",
				r.personality, planDir, actionDir, reviewPath,
			)

			if err := runOpenCode(prompt, windowName); err != nil {
				return modes.ModeErrorMsg{Error: fmt.Errorf("failed to start %s review: %w", r.name, err)}
			}

			// Small delay between spawning to avoid race conditions
			time.Sleep(500 * time.Millisecond)
		}

		return pollTickMsg{}
	}
}

// startConsolidation spawns the orchestrator to apply feedback
func (m *Mode) startConsolidation() tea.Cmd {
	return func() tea.Msg {
		ctx := m.Context()

		m.killAllWindows()
		m.windowName = fmt.Sprintf("consolidation-%d", time.Now().Unix())
		if err := createTmuxWindow(m.windowName); err != nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("failed to create tmux window: %w", err)}
		}

		planDir := ctx.Workflow.PlanDir()
		actionDir := ctx.Workflow.ActionDir()
		markerPath := ctx.Workflow.ReviewsAppliedPath()

		prompt := fmt.Sprintf(
			"You are the ORCHESTRATOR consolidating feedback from multiple reviewers. "+
				"Read all documents: "+
				"- Original plan: %s (COMMISSION.md, ARCHITECTURE.md, CONVENTIONS.md) "+
				"- Action plan: %s (MODULES.md, PLAN.md) "+
				"- Staff review: %s/STAFF_REVIEW.md "+
				"- Pragmatist review: %s/REVIEW_PRAGMATIST.md "+
				"- Simplifier review: %s/REVIEW_SIMPLIFIER.md "+
				"- User Advocate review: %s/REVIEW_USER_ADVOCATE.md "+
				"- Skeptic review: %s/REVIEW_SKEPTIC.md "+
				"Synthesize the feedback. Identify: "+
				"1. Common themes across reviewers "+
				"2. Critical issues that must be addressed "+
				"3. Suggestions to incorporate "+
				"4. Feedback to note but not act on (with reasoning) "+
				"Update MODULES.md and PLAN.md with improvements based on the feedback. "+
				"When done, create an empty marker file at %s to signal completion. "+
				"Do not end until the marker file exists.",
			planDir, actionDir, actionDir, actionDir, actionDir, actionDir, actionDir, markerPath,
		)

		if err := runOpenCode(prompt, m.windowName); err != nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("failed to start consolidation: %w", err)}
		}

		return pollTickMsg{}
	}
}

// startBeadCreation runs bd init and creates beads from modules and plan
func (m *Mode) startBeadCreation() tea.Cmd {
	return func() tea.Msg {
		ctx := m.Context()

		projectDir := ctx.Config.ProjectDir
		if err := ensureBeadsInitialized(projectDir); err != nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("failed to initialize beads: %w", err)}
		}

		m.killAllWindows()
		m.windowName = fmt.Sprintf("bead-creation-%d", time.Now().Unix())
		if err := createTmuxWindow(m.windowName); err != nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("failed to create tmux window: %w", err)}
		}

		actionDir := ctx.Workflow.ActionDir()
		markerPath := ctx.Workflow.BeadsCreatedPath()

		prompt := fmt.Sprintf(
			"You are setting up work tracking with beads (bd). "+
				"IMPORTANT: Use 'bd' for task tracking. Read AGENTS.md for instructions. "+
				"First, initialize beads in the project directory: "+
				"cd %s && bd init "+
				"Then read the planning documents: "+
				"- %s/MODULES.md - Contains the top-level parallelizable modules "+
				"- %s/PLAN.md - Contains the implementation plan with tasks "+
				"Create beads for tracking: "+
				"1. For each MODULE, create an epic bead: bd create \"<module name>\" -t epic -p 1 "+
				"2. For each task in PLAN.md, create a child bead under its parent module: "+
				"   bd create \"<task name>\" --parent <module-bead-id> "+
				"Make sure each plan task is linked to the correct module. "+
				"When all beads are created, create an empty marker file at %s to signal completion. "+
				"Run 'bd list' at the end to verify the structure. "+
				"Do not end until the marker file exists.",
			projectDir, actionDir, actionDir, markerPath,
		)

		if err := runOpenCode(prompt, m.windowName); err != nil {
			return modes.ModeErrorMsg{Error: fmt.Errorf("failed to start bead creation: %w", err)}
		}

		return pollTickMsg{}
	}
}

// pollForCompletion checks if the required files exist
func (m *Mode) pollForCompletion() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		ctx := m.Context()
		wf := ctx.Workflow

		switch m.phase {
		case phaseAnchorDocs:
			if wf.AnchorDocsComplete() {
				return anchorDocsCompleteMsg{}
			}
		case phaseActionPlan:
			if fileExists(wf.ModulesPath()) && fileExists(wf.ActionPlanPath()) {
				return actionPlanCompleteMsg{}
			}
		case phaseStaffReview:
			if fileExists(wf.StaffReviewPath()) {
				return staffReviewCompleteMsg{}
			}
		case phaseStaffIncorporation:
			if fileExists(wf.StaffFeedbackAppliedPath()) {
				return staffFeedbackAppliedMsg{}
			}
		case phaseParallelReviews:
			if wf.AllReviewsComplete() {
				return parallelReviewsCompleteMsg{}
			}
		case phaseConsolidation:
			if fileExists(wf.ReviewsAppliedPath()) {
				return consolidationCompleteMsg{}
			}
		case phaseBeadCreation:
			if fileExists(wf.BeadsCreatedPath()) {
				return beadsCreatedMsg{}
			}
		case phasePlanChat:
			if fileExists(wf.PlanChatReadyPath()) {
				return planChatCompleteMsg{}
			}
		}

		return pollTickMsg{}
	})
}

// killWindow cleans up a single tmux window
func (m *Mode) killWindow() {
	if m.windowName != "" {
		killTmuxWindow(m.windowName)
		m.windowName = ""
	}
}

// killAllWindows cleans up all tmux windows
func (m *Mode) killAllWindows() {
	m.killWindow()
	for _, name := range m.windowNames {
		if name != "" {
			killTmuxWindow(name)
		}
	}
	m.windowNames = nil
}

func (m *Mode) clearPlanChatMarkers() {
	ctx := m.Context()
	if ctx == nil {
		return
	}
	wf := ctx.Workflow
	removeFileIfExists(wf.PlanChatReadyPath())
	removeFileIfExists(wf.PlanChatActivePath())
}

func removeFileIfExists(path string) {
	if path == "" {
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		// Ignore errors removing markers; they'll be recreated when needed
	}
}

// Helper functions for tmux and opencode

func createTmuxWindow(name string) error {
	cmd := exec.Command("tmux", "new-window", "-n", name)
	return cmd.Run()
}

func killTmuxWindow(name string) error {
	cmd := exec.Command("tmux", "kill-window", "-t", name)
	return cmd.Run()
}

func runOpenCode(prompt string, windowName string) error {
	escapedPrompt := strings.ReplaceAll(prompt, `"`, `\"`)
	escapedPrompt = strings.ReplaceAll(escapedPrompt, "\n", " ")
	opencodeCmd := fmt.Sprintf(`opencode --prompt "%s"`, escapedPrompt)
	cmd := exec.Command("tmux", "send-keys", "-t", windowName, opencodeCmd, "Enter")
	return cmd.Run()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func ensureBeadsInitialized(projectDir string) error {
	needsInit, err := beadsInitRequired(projectDir)
	if err != nil {
		return err
	}
	if !needsInit {
		return nil
	}
	return runBeadsInit(projectDir)
}

func beadsInitRequired(projectDir string) (bool, error) {
	agentsPath := filepath.Join(projectDir, "AGENTS.md")
	if _, err := os.Stat(agentsPath); err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("checking AGENTS.md: %w", err)
	}

	beadsDir := filepath.Join(projectDir, ".beads")
	entries, err := os.ReadDir(beadsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("reading .beads: %w", err)
	}
	if len(entries) == 0 {
		return true, nil
	}
	return false, nil
}

func runBeadsInit(projectDir string) error {
	cmd := exec.Command("bd", "init")
	cmd.Dir = projectDir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		trimmed := strings.TrimSpace(out.String())
		if trimmed != "" {
			return fmt.Errorf("bd init failed: %s: %w", trimmed, err)
		}
		return fmt.Errorf("bd init failed: %w", err)
	}
	return nil
}
