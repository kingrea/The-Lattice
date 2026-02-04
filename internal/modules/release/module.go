package release

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yourusername/lattice/internal/artifact"
	"github.com/yourusername/lattice/internal/module"
	"github.com/yourusername/lattice/internal/modules/runtime"
)

const (
	moduleID      = "release"
	moduleVersion = "1.0.0"
	defaultBDWait = 15 * time.Second
)

// Option customizes the release module.
type Option func(*Module)

// Module orchestrates packaging and release notes emission.
type Module struct {
	*module.Base
	now   func() time.Time
	beads beadLister
}

// Register installs the release module factory.
func Register(reg *module.Registry) {
	if reg == nil {
		return
	}
	reg.MustRegister(moduleID, func(module.Config) (module.Module, error) {
		return New(), nil
	})
}

// New constructs a release module with optional overrides.
func New(opts ...Option) *Module {
	info := module.Info{
		ID:          moduleID,
		Name:        "Finalize Release",
		Description: "Synthesizes release notes, packages artifacts, and clears runtime state.",
		Version:     moduleVersion,
		Concurrency: module.ConcurrencyProfile{Exclusive: true},
	}
	base := module.NewBase(info)
	base.SetInputs(
		artifact.WorkCompleteMarker,
		artifact.WorkLogDoc,
		artifact.WorkersJSON,
		artifact.OrchestratorState,
	)
	base.SetOutputs(
		artifact.ReleaseNotesDoc,
		artifact.ReleasePackagesDir,
		artifact.AgentsReleasedMarker,
		artifact.CleanupDoneMarker,
		artifact.OrchestratorReleasedMarker,
	)
	mod := &Module{
		Base:  &base,
		now:   time.Now,
		beads: execBeadLister{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(mod)
		}
	}
	return mod
}

// WithClock overrides the module timestamp source (tests).
func WithClock(clock func() time.Time) Option {
	return func(m *Module) {
		if clock != nil {
			m.now = clock
		}
	}
}

// WithBeadLister injects a ready-bead provider (tests).
func WithBeadLister(l beadLister) Option {
	return func(m *Module) {
		if l != nil {
			m.beads = l
		}
	}
}

// Run orchestrates release packaging.
func (m *Module) Run(ctx *module.ModuleContext) (module.Result, error) {
	if err := runtime.ValidateContext(moduleID, ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if missing, err := m.missingInput(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	} else if missing != "" {
		return module.Result{Status: module.StatusNeedsInput, Message: fmt.Sprintf("waiting for %s", missing)}, nil
	}
	if pending, err := m.refinementPending(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	} else if pending {
		return module.Result{Status: module.StatusNeedsInput, Message: "pending refinement follow-ups"}, nil
	}
	if done, err := m.IsComplete(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	} else if done {
		return module.Result{Status: module.StatusNoOp, Message: "release already finalized"}, nil
	}
	releaseDir := ctx.Workflow.ReleaseDir()
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		return module.Result{Status: module.StatusFailed}, fmt.Errorf("%s: ensure release dir: %w", moduleID, err)
	}
	if err := ctx.Artifacts.Write(artifact.ReleasePackagesDir, nil, artifact.Metadata{}); err != nil {
		return module.Result{Status: module.StatusFailed}, fmt.Errorf("%s: ensure packages dir: %w", moduleID, err)
	}
	packagePath, err := m.createReleasePackage(ctx)
	if err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	beads, beadWarning := m.listOutstandingBeads()
	workLogBody, err := m.readDocumentBody(artifact.WorkLogDoc.Path(ctx.Workflow))
	if err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	workers, err := m.readWorkerNames(artifact.WorkersJSON.Path(ctx.Workflow))
	if err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	orchestratorName, _ := m.readOrchestratorName(artifact.OrchestratorState.Path(ctx.Workflow))
	auditBody, _ := m.readDocumentBody(artifact.AuditSynthesisDoc.Path(ctx.Workflow))
	releaseBody := m.renderReleaseNotes(workLogBody, auditBody, workers, orchestratorName, beads, filepath.Base(packagePath), beadWarning)
	if err := m.writeReleaseNotes(ctx, []byte(releaseBody)); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if err := m.archiveWorkLog(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if err := m.archiveWorkerRoster(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if err := m.cleanupRuntime(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	if err := m.writeMarkers(ctx); err != nil {
		return module.Result{Status: module.StatusFailed}, err
	}
	message := fmt.Sprintf("package %s", filepath.Base(packagePath))
	if beadWarning != "" {
		message = fmt.Sprintf("%s (warning: %s)", message, beadWarning)
	}
	return module.Result{Status: module.StatusCompleted, Message: message}, nil
}

// IsComplete returns true when the orchestrator release marker exists.
func (m *Module) IsComplete(ctx *module.ModuleContext) (bool, error) {
	if err := runtime.ValidateContext(moduleID, ctx); err != nil {
		return false, err
	}
	ready, err := runtime.EnsureMarker(ctx, moduleID, moduleVersion, artifact.OrchestratorReleasedMarker)
	return ready, err
}

func (m *Module) missingInput(ctx *module.ModuleContext) (string, error) {
	for _, ref := range m.Inputs() {
		result, err := ctx.Artifacts.Check(ref)
		if err != nil {
			return "", fmt.Errorf("%s: check %s: %w", moduleID, ref.ID, err)
		}
		if result.State != artifact.StateReady {
			return ref.Name, nil
		}
	}
	return "", nil
}

func (m *Module) refinementPending(ctx *module.ModuleContext) (bool, error) {
	result, err := ctx.Artifacts.Check(artifact.RefinementNeededMarker)
	if err != nil {
		return false, fmt.Errorf("%s: check refinement marker: %w", moduleID, err)
	}
	return result.State == artifact.StateReady, nil
}

func (m *Module) listOutstandingBeads() ([]beadSummary, string) {
	if m.beads == nil {
		return nil, "ready queue unavailable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultBDWait)
	defer cancel()
	issues, err := m.beads.Ready(ctx)
	if err != nil {
		return nil, err.Error()
	}
	if len(issues) == 0 {
		return nil, ""
	}
	sorted := append([]beadSummary(nil), issues...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Priority == sorted[j].Priority {
			return strings.ToLower(sorted[i].ID) < strings.ToLower(sorted[j].ID)
		}
		return sorted[i].Priority < sorted[j].Priority
	})
	return sorted, ""
}

func (m *Module) writeReleaseNotes(ctx *module.ModuleContext, body []byte) error {
	meta := artifact.Metadata{
		ArtifactID: artifact.ReleaseNotesDoc.ID,
		ModuleID:   moduleID,
		Version:    moduleVersion,
		Workflow:   ctx.Workflow.Dir(),
	}
	runtime.WithInputs(m.Inputs()...)(&meta)
	runtime.WithFingerprint(artifact.ReleaseNotesDoc, fingerprint(body))(&meta)
	if err := ctx.Artifacts.Write(artifact.ReleaseNotesDoc, body, meta); err != nil {
		return fmt.Errorf("%s: write release notes: %w", moduleID, err)
	}
	return nil
}

func (m *Module) createReleasePackage(ctx *module.ModuleContext) (string, error) {
	root := artifact.ReleasePackagesDir.Path(ctx.Workflow)
	if root == "" {
		return "", fmt.Errorf("%s: release packages path unavailable", moduleID)
	}
	timestamp := m.now().UTC().Format("20060102-150405")
	dest := filepath.Join(root, timestamp)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", fmt.Errorf("%s: create package dir: %w", moduleID, err)
	}
	copyFiles := []struct{ src, dst string }{
		{artifact.WorkLogDoc.Path(ctx.Workflow), filepath.Join(dest, "work-log.md")},
		{artifact.OrchestratorState.Path(ctx.Workflow), filepath.Join(dest, "orchestrator.json")},
		{artifact.WorkersJSON.Path(ctx.Workflow), filepath.Join(dest, "workers.json")},
		{artifact.AuditSynthesisDoc.Path(ctx.Workflow), filepath.Join(dest, "audit", "SYNTHESIS.md")},
	}
	for _, entry := range copyFiles {
		if err := copyFileIfExists(entry.src, entry.dst); err != nil {
			return "", err
		}
	}
	if err := copyDirIfExists(artifact.AuditDirectory.Path(ctx.Workflow), filepath.Join(dest, "audit")); err != nil {
		return "", err
	}
	if err := copyDirIfExists(ctx.Config.LogsDir(), filepath.Join(dest, "logs")); err != nil {
		return "", err
	}
	if err := copyDirIfExists(ctx.Config.WorktreeDir(), filepath.Join(dest, "worktree")); err != nil {
		return "", err
	}
	return dest, nil
}

func (m *Module) archiveWorkLog(ctx *module.ModuleContext) error {
	path := artifact.WorkLogDoc.Path(ctx.Workflow)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("%s: read work log: %w", moduleID, err)
	}
	name := fmt.Sprintf("work-log-%s.md", m.now().UTC().Format("20060102-150405"))
	dest := filepath.Join(ctx.Workflow.ReleaseDir(), name)
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return fmt.Errorf("%s: archive work log: %w", moduleID, err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s: clear work log: %w", moduleID, err)
	}
	return nil
}

func (m *Module) archiveWorkerRoster(ctx *module.ModuleContext) error {
	path := ctx.Config.WorkerListPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("%s: read workers manifest: %w", moduleID, err)
	}
	name := fmt.Sprintf("workers-%s.json", m.now().UTC().Format("20060102-150405"))
	dest := filepath.Join(ctx.Workflow.ReleaseDir(), name)
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return fmt.Errorf("%s: archive workers: %w", moduleID, err)
	}
	if err := os.WriteFile(path, []byte("[]\n"), 0o644); err != nil {
		return fmt.Errorf("%s: reset workers: %w", moduleID, err)
	}
	return nil
}

func (m *Module) cleanupRuntime(ctx *module.ModuleContext) error {
	if err := resetDirectory(ctx.Config.LogsDir()); err != nil {
		return err
	}
	if err := resetDirectory(ctx.Config.WorktreeDir()); err != nil {
		return err
	}
	if err := os.RemoveAll(ctx.Config.AgentsDir()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s: remove agents dir: %w", moduleID, err)
	}
	if err := os.Remove(ctx.Workflow.OrchestratorPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s: remove orchestrator state: %w", moduleID, err)
	}
	return nil
}

func (m *Module) writeMarkers(ctx *module.ModuleContext) error {
	markers := []artifact.ArtifactRef{
		artifact.AgentsReleasedMarker,
		artifact.CleanupDoneMarker,
		artifact.OrchestratorReleasedMarker,
	}
	for _, ref := range markers {
		if err := ctx.Artifacts.Write(ref, nil, artifact.Metadata{}); err != nil {
			return fmt.Errorf("%s: write %s: %w", moduleID, ref.ID, err)
		}
	}
	return nil
}

func (m *Module) renderReleaseNotes(workLog, audit string, workers []string, orchestrator string, beads []beadSummary, packageName, warning string) string {
	var b strings.Builder
	timestamp := m.now().UTC().Format(time.RFC3339)
	b.WriteString("# Release Notes\n\n")
	b.WriteString(fmt.Sprintf("Generated at %s UTC by module %s/%s.\n\n", timestamp, moduleID, moduleVersion))
	b.WriteString("## Delivery Snapshot\n\n")
	if orchestrator != "" {
		b.WriteString(fmt.Sprintf("- Orchestrator: %s\n", orchestrator))
	}
	b.WriteString(fmt.Sprintf("- Workers: %d\n", len(workers)))
	b.WriteString(fmt.Sprintf("- Package: %s\n", packageName))
	if warning != "" {
		b.WriteString(fmt.Sprintf("- Release warning: %s\n", warning))
	}
	b.WriteString("\n## Active Roster\n\n")
	if len(workers) == 0 {
		b.WriteString("_No active workers registered._\n")
	} else {
		sorted := append([]string(nil), workers...)
		sort.Strings(sorted)
		for _, name := range sorted {
			b.WriteString(fmt.Sprintf("- %s\n", name))
		}
	}
	b.WriteString("\n## Outstanding Beads\n\n")
	if len(beads) == 0 {
		b.WriteString("All tracked beads delivered.\n")
	} else {
		for _, bead := range beads {
			b.WriteString(fmt.Sprintf("- %s · %s (P%d %s)\n", bead.ID, bead.Title, bead.Priority, strings.ToLower(strings.TrimSpace(bead.Status))))
		}
	}
	b.WriteString("\n## Work Log\n\n")
	if strings.TrimSpace(workLog) == "" {
		b.WriteString("_Work log not available._\n")
	} else {
		b.WriteString(workLog)
		if !strings.HasSuffix(workLog, "\n") {
			b.WriteString("\n")
		}
	}
	if strings.TrimSpace(audit) != "" {
		b.WriteString("\n## Audit Highlights\n\n")
		b.WriteString(audit)
		if !strings.HasSuffix(audit, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func (m *Module) readDocumentBody(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("%s: read document %s: %w", moduleID, filepath.Base(path), err)
	}
	meta, body, err := artifact.ParseFrontMatter(data)
	if err != nil {
		if errors.Is(err, artifact.ErrMissingFrontMatter) || errors.Is(err, artifact.ErrMalformedFrontMatter) {
			return string(data), nil
		}
		return "", fmt.Errorf("%s: parse %s: %w", moduleID, filepath.Base(path), err)
	}
	_ = meta
	return string(body), nil
}

func (m *Module) readWorkerNames(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: read workers json: %w", moduleID, err)
	}
	var payload struct {
		Workers []struct {
			Name string `json:"name"`
		} `json:"workers"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("%s: parse workers json: %w", moduleID, err)
	}
	result := make([]string, 0, len(payload.Workers))
	for _, worker := range payload.Workers {
		name := strings.TrimSpace(worker.Name)
		if name != "" {
			result = append(result, name)
		}
	}
	return result, nil
}

func (m *Module) readOrchestratorName(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var payload struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", err
	}
	return strings.TrimSpace(payload.Name), nil
}

type beadLister interface {
	Ready(context.Context) ([]beadSummary, error)
}

type beadSummary struct {
	ID       string
	Title    string
	Priority int
	Status   string
}

type execBeadLister struct{}

func (execBeadLister) Ready(ctx context.Context) ([]beadSummary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, "bd", "ready", "--json")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var issues []struct {
		ID       string      `json:"id"`
		Title    string      `json:"title"`
		Priority json.Number `json:"priority"`
		Status   string      `json:"status"`
	}
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, err
	}
	result := make([]beadSummary, 0, len(issues))
	for _, issue := range issues {
		priority, _ := issue.Priority.Int64()
		result = append(result, beadSummary{
			ID:       strings.TrimSpace(issue.ID),
			Title:    strings.TrimSpace(issue.Title),
			Priority: int(priority),
			Status:   strings.TrimSpace(issue.Status),
		})
	}
	return result, nil
}

func copyFileIfExists(src, dst string) error {
	if strings.TrimSpace(src) == "" || strings.TrimSpace(dst) == "" {
		return nil
	}
	info, err := os.Stat(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("release: stat %s: %w", src, err)
	}
	if info.IsDir() {
		return copyDirIfExists(src, dst)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("release: prepare %s: %w", dst, err)
	}
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("release: open %s: %w", src, err)
	}
	defer srcFile.Close()
	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("release: create %s: %w", dst, err)
	}
	defer dstFile.Close()
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("release: copy %s: %w", filepath.Base(src), err)
	}
	return nil
}

func copyDirIfExists(src, dst string) error {
	if strings.TrimSpace(src) == "" || strings.TrimSpace(dst) == "" {
		return nil
	}
	info, err := os.Stat(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("release: stat dir %s: %w", src, err)
	}
	if !info.IsDir() {
		return copyFileIfExists(src, dst)
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFileIfExists(path, target)
	})
}

func resetDirectory(path string) error {
	if path == "" {
		return nil
	}
	if err := os.RemoveAll(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("release: reset %s: %w", path, err)
	}
	return os.MkdirAll(path, 0o755)
}

func fingerprint(body []byte) string {
	sum := sha256.Sum256(body)
	return fmt.Sprintf("%x", sum[:])
}
