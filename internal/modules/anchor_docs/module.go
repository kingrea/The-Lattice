package anchor_docs

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/yourusername/lattice/internal/artifact"
	"github.com/yourusername/lattice/internal/module"
	"github.com/yourusername/lattice/internal/skills"
)

const (
	moduleID      = "anchor-docs"
	moduleVersion = "1.0.0"
)

// AnchorDocsModule drives the lattice-planning skill to create the three anchor
// documents and ensures they carry lattice provenance metadata.
type AnchorDocsModule struct {
	*module.Base
	windowName string
}

// Register installs the module factory into the provided registry.
func Register(reg *module.Registry) {
	if reg == nil {
		return
	}
	reg.MustRegister(moduleID, func(module.Config) (module.Module, error) {
		return New(), nil
	})
}

// New constructs the module with its IO contracts declared.
func New() *AnchorDocsModule {
	info := module.Info{
		ID:          moduleID,
		Name:        "Create Anchor Documents",
		Description: "Generates COMMISSION.md, ARCHITECTURE.md, and CONVENTIONS.md via the lattice-planning skill.",
		Version:     moduleVersion,
	}
	base := module.NewBase(info)
	base.SetOutputs(
		artifact.CommissionDoc,
		artifact.ArchitectureDoc,
		artifact.ConventionsDoc,
	)
	return &AnchorDocsModule{Base: &base}
}

// Run launches the planning skill in a tmux window if the artifacts are not
// already present.
func (m *AnchorDocsModule) Run(ctx *module.ModuleContext) (module.Result, error) {
	if err := validateContext(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if complete, err := m.IsComplete(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	} else if complete {
		return module.Result{Status: module.StatusNoOp, Message: "anchor docs already exist"}, nil
	}
	if m.windowName != "" {
		return module.Result{Status: module.StatusNeedsInput, Message: fmt.Sprintf("anchor docs running in %s", m.windowName)}, nil
	}
	window := fmt.Sprintf("anchor-docs-%d", time.Now().Unix())
	if err := createTmuxWindow(window, ctx.Config.ProjectDir); err != nil {
		return module.Result{Status: module.StatusFailed}, fmt.Errorf("anchor-docs: create tmux window: %w", err)
	}
	skillPath, err := skills.Ensure(ctx.Config.SkillsDir(), skills.LatticePlanning)
	if err != nil {
		killTmuxWindow(window)
		return module.Result{Status: module.StatusFailed}, fmt.Errorf("anchor-docs: ensure planning skill: %w", err)
	}
	prompt := fmt.Sprintf(
		"Load and execute the skill at %s. Create the three anchor documents (COMMISSION.md, ARCHITECTURE.md, CONVENTIONS.md) in %s. Work with the user to gather requirements and make decisions. Write each file as soon as that phase completes. Do not end until all three files exist.",
		skillPath,
		ctx.Workflow.PlanDir(),
	)
	if err := runOpenCode(window, prompt); err != nil {
		killTmuxWindow(window)
		return module.Result{Status: module.StatusFailed}, fmt.Errorf("anchor-docs: launch opencode: %w", err)
	}
	m.windowName = window
	return module.Result{Status: module.StatusNeedsInput, Message: fmt.Sprintf("anchor docs running in %s", window)}, nil
}

// IsComplete checks that all output artifacts exist with valid metadata.
func (m *AnchorDocsModule) IsComplete(ctx *module.ModuleContext) (bool, error) {
	if err := validateContext(ctx); err != nil {
		return false, err
	}
	for _, ref := range m.Outputs() {
		ready, err := m.ensureArtifact(ctx, ref)
		if err != nil {
			return false, err
		}
		if !ready {
			return false, nil
		}
	}
	m.stopSession()
	return true, nil
}

func (m *AnchorDocsModule) ensureArtifact(ctx *module.ModuleContext, ref artifact.ArtifactRef) (bool, error) {
	result, err := ctx.Artifacts.Check(ref)
	if err != nil {
		return false, fmt.Errorf("anchor-docs: check %s: %w", ref.ID, err)
	}
	switch result.State {
	case artifact.StateReady:
		if result.Metadata == nil || result.Metadata.ModuleID != moduleID || result.Metadata.Version != moduleVersion {
			if err := m.writeMetadata(ctx, ref); err != nil {
				return false, err
			}
			return false, nil
		}
		return true, nil
	case artifact.StateMissing:
		return false, nil
	case artifact.StateInvalid:
		if err := m.writeMetadata(ctx, ref); err != nil {
			return false, err
		}
		return false, nil
	case artifact.StateError:
		if result.Err != nil {
			return false, fmt.Errorf("anchor-docs: %s: %w", ref.ID, result.Err)
		}
		return false, fmt.Errorf("anchor-docs: %s encountered an unknown error", ref.ID)
	default:
		return false, nil
	}
}

func (m *AnchorDocsModule) writeMetadata(ctx *module.ModuleContext, ref artifact.ArtifactRef) error {
	path := ref.Path(ctx.Workflow)
	if path == "" {
		return fmt.Errorf("anchor-docs: unable to resolve path for %s", ref.ID)
	}
	body, err := readDocumentBody(path)
	if err != nil {
		return fmt.Errorf("anchor-docs: read %s: %w", ref.ID, err)
	}
	meta := artifact.Metadata{
		ArtifactID: ref.ID,
		ModuleID:   moduleID,
		Version:    moduleVersion,
		Workflow:   ctx.Workflow.Dir(),
	}
	if err := ctx.Artifacts.Write(ref, body, meta); err != nil {
		return fmt.Errorf("anchor-docs: write %s: %w", ref.ID, err)
	}
	return nil
}

func (m *AnchorDocsModule) stopSession() {
	if m.windowName == "" {
		return
	}
	killTmuxWindow(m.windowName)
	m.windowName = ""
}

func validateContext(ctx *module.ModuleContext) error {
	if ctx == nil {
		return fmt.Errorf("anchor-docs: context is nil")
	}
	if ctx.Config == nil {
		return fmt.Errorf("anchor-docs: config is required")
	}
	if ctx.Workflow == nil {
		return fmt.Errorf("anchor-docs: workflow is required")
	}
	if ctx.Artifacts == nil {
		return fmt.Errorf("anchor-docs: artifact store is required")
	}
	return nil
}

func readDocumentBody(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	if _, body, err := artifact.ParseFrontMatter(data); err == nil {
		return body, nil
	}
	return data, nil
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
