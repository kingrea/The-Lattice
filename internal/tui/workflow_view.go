package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kingrea/The-Lattice/internal/config"
	"github.com/kingrea/The-Lattice/internal/module"
	"github.com/kingrea/The-Lattice/internal/modules"
	"github.com/kingrea/The-Lattice/internal/workflow"
	"github.com/kingrea/The-Lattice/internal/workflow/engine"
	"github.com/kingrea/The-Lattice/internal/workflow/scheduler"
	"github.com/kingrea/The-Lattice/plugins"
)

const engineRefreshInterval = 5 * time.Second

var (
	labelStyleReady   = lipgloss.NewStyle().Foreground(lipgloss.Color("#4CAF50")).Bold(true)
	labelStyleBlocked = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Bold(true)
	labelStyleRunning = lipgloss.NewStyle().Foreground(lipgloss.Color("#5B8DEF")).Bold(true)
	labelStyleGate    = lipgloss.NewStyle().Foreground(lipgloss.Color("#F7B801")).Bold(true)
	labelStyleSkipped = lipgloss.NewStyle().Foreground(lipgloss.Color("#999999"))
	labelStyleDefault = lipgloss.NewStyle().Foreground(lipgloss.Color("#CCCCCC"))
	detailTextStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#A0AEC0"))
)

type workflowView struct {
	app             *App
	moduleCtx       *module.ModuleContext
	registry        *module.Registry
	engine          *engine.Engine
	workflowID      string
	definition      workflow.WorkflowDefinition
	moduleRefs      map[string]workflow.ModuleRef
	state           engine.State
	stateLoaded     bool
	err             error
	selection       int
	running         map[string]struct{}
	manualGates     map[string]scheduler.ManualGateState
	targets         []string
	loader          WorkflowDefinitionLoader
	registryFactory func(*config.Config) (*module.Registry, error)
	finished        bool
}

type moduleLabel struct {
	text  string
	style lipgloss.Style
}

type workflowInitMsg struct {
	state engine.State
	err   error
}

type workflowStateMsg struct {
	state engine.State
	err   error
}

type engineRefreshRequest struct{}

type moduleRunFinishedMsg struct {
	id     string
	result module.Result
	err    error
}

type workClaimMsg struct {
	result engine.ClaimResult
	err    error
}

type workflowFinishedMsg struct {
	WorkflowID string
	RunID      string
	Status     engine.EngineStatus
	Reason     string
}

func newWorkflowView(app *App, workflowID string) *workflowView {
	id := strings.TrimSpace(workflowID)
	if id == "" {
		id = strings.TrimSpace(app.config.DefaultWorkflow())
	}
	if id == "" {
		id = "commission-work"
	}
	view := &workflowView{
		app:         app,
		workflowID:  id,
		running:     map[string]struct{}{},
		manualGates: map[string]scheduler.ManualGateState{},
	}
	view.loader = app.workflowLoader
	view.registryFactory = app.registryFactory
	return view
}

func (v *workflowView) Init(resume bool) tea.Cmd {
	return func() tea.Msg {
		state, err := v.bootstrap(resume)
		return workflowInitMsg{state: state, err: err}
	}
}

func (v *workflowView) Update(msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case workflowInitMsg:
		if m.err != nil {
			v.err = m.err
			v.setStatus(fmt.Sprintf("Workflow error: %v", m.err))
			return nil
		}
		v.err = nil
		v.stateLoaded = true
		cmd := v.applyState(m.state)
		v.setStatus("Workflow engine ready")
		if v.finished {
			return cmd
		}
		refresh := v.scheduleRefresh()
		if cmd != nil && refresh != nil {
			return tea.Batch(cmd, refresh)
		}
		if cmd != nil {
			return cmd
		}
		return refresh
	case workflowStateMsg:
		if m.err != nil {
			v.err = m.err
			v.setStatus(fmt.Sprintf("Engine update failed: %v", m.err))
			return nil
		}
		v.err = nil
		return v.applyState(m.state)
	case engineRefreshRequest:
		if !v.stateLoaded || v.finished {
			return nil
		}
		refresh := v.refreshEngineState()
		schedule := v.scheduleRefresh()
		if refresh != nil && schedule != nil {
			return tea.Batch(refresh, schedule)
		}
		if refresh != nil {
			return refresh
		}
		return schedule
	case moduleRunFinishedMsg:
		return v.handleModuleRunFinished(m)
	case workClaimMsg:
		if m.err != nil {
			v.err = m.err
			v.setStatus(fmt.Sprintf("Claim failed: %v", m.err))
			return nil
		}
		v.err = nil
		cmd := v.applyState(m.result.State)
		if len(m.result.Claims) == 0 {
			v.setStatus("No runnable modules satisfied the request")
			return cmd
		}
		launch := v.launchClaims(m.result.Claims)
		if cmd != nil && launch != nil {
			return tea.Batch(cmd, launch)
		}
		if cmd != nil {
			return cmd
		}
		return launch
	case tea.KeyMsg:
		return v.handleKeyMsg(m)
	default:
		return nil
	}
}

func (v *workflowView) View() string {
	if v.err != nil {
		return fmt.Sprintf("Workflow error: %v", v.err)
	}
	if !v.stateLoaded {
		return "Preparing workflow engine…"
	}
	statusLine := fmt.Sprintf("Workflow: %s · Status: %s", v.state.WorkflowID, strings.Title(string(v.state.Status)))
	if v.state.StatusReason != "" {
		statusLine += fmt.Sprintf(" · %s", v.state.StatusReason)
	}
	lines := []string{statusLine, fmt.Sprintf("Ready modules: %d", len(v.state.Runnable)), ""}
	for i, node := range v.state.Nodes {
		lines = append(lines, v.renderModuleLine(i, node))
		if i == v.selection {
			lines = append(lines, v.renderModuleDetails(node))
		}
	}
	lines = append(lines,
		"",
		"enter=run  r=refresh  s=skip optional  g=toggle gate  a=approve gate",
		"esc=back to menu",
	)
	return strings.Join(lines, "\n")
}

func (v *workflowView) renderModuleLine(idx int, node engine.ModuleStatus) string {
	indicator := " "
	if idx == v.selection {
		indicator = ">"
	}
	name := node.Name
	if strings.TrimSpace(name) == "" {
		name = node.ID
	}
	labelSpecs := v.moduleLabelSpecs(node)
	if len(labelSpecs) == 0 {
		labelSpecs = []moduleLabel{{text: "Unknown", style: labelStyleDefault}}
	}
	rendered := make([]string, 0, len(labelSpecs))
	for _, spec := range labelSpecs {
		rendered = append(rendered, spec.style.Render(spec.text))
	}
	return fmt.Sprintf("%s %s · [%s]", indicator, name, strings.Join(rendered, ", "))
}

func (v *workflowView) renderModuleDetails(node engine.ModuleStatus) string {
	var details []string
	if node.Description != "" {
		details = append(details, node.Description)
	}
	if len(node.BlockedBy) > 0 {
		details = append(details, fmt.Sprintf("Blocked by: %s", strings.Join(node.BlockedBy, ", ")))
	}
	if run, ok := v.state.Runs[node.ID]; ok {
		runLine := fmt.Sprintf("Last run: %s", run.Status)
		if run.Message != "" {
			runLine += fmt.Sprintf(" · %s", run.Message)
		}
		if run.Error != "" {
			runLine += fmt.Sprintf(" · error: %s", run.Error)
		}
		details = append(details, runLine)
	}
	if len(details) == 0 {
		return detailTextStyle.Render("  no additional details")
	}
	body := "  " + strings.Join(details, "\n  ")
	return detailTextStyle.Render(body)
}

func (v *workflowView) moduleLabelSpecs(node engine.ModuleStatus) []moduleLabel {
	var specs []moduleLabel
	add := func(text string, style lipgloss.Style) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		for _, existing := range specs {
			if existing.text == text {
				return
			}
		}
		specs = append(specs, moduleLabel{text: text, style: style})
	}
	stateText := friendlyLabel(string(node.State))
	add(stateText, labelStyleForState(string(node.State)))
	if v.isRunnable(node.ID) {
		add("Ready", labelStyleReady)
	}
	if _, ok := v.running[node.ID]; ok {
		add("Running", labelStyleRunning)
	}
	if gate, ok := v.manualGates[node.ID]; ok && gate.Required {
		label := "Gate Pending"
		style := labelStyleGate
		if gate.Approved {
			label = "Gate Approved"
			style = labelStyleReady
		}
		add(label, style)
	}
	if skip, ok := v.state.Skipped[node.ID]; ok {
		detail := strings.TrimSpace(skip.Detail)
		label := "Skipped"
		if detail != "" {
			label = fmt.Sprintf("Skipped (%s)", friendlyLabel(detail))
		}
		add(label, labelStyleSkipped)
	}
	return specs
}

func labelStyleForState(state string) lipgloss.Style {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "ready":
		return labelStyleReady
	case "blocked":
		return labelStyleBlocked
	case "running":
		return labelStyleRunning
	case "skipped":
		return labelStyleSkipped
	default:
		return labelStyleDefault
	}
}

func friendlyLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer("_", " ", "-", " ")
	words := strings.Fields(replacer.Replace(strings.ToLower(value)))
	if len(words) == 0 {
		return ""
	}
	for i, word := range words {
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}

func (v *workflowView) handleKeyMsg(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "up", "k":
		if v.selection > 0 {
			v.selection--
		}
	case "down", "j":
		if v.selection < len(v.state.Nodes)-1 {
			v.selection++
		}
	case "enter":
		return v.runSelectedModule()
	case "r":
		return v.refreshEngineState()
	case "s":
		if v.skipSelectedModule() {
			return v.syncRuntime()
		}
	case "g":
		if v.toggleGateRequirement() {
			return v.syncRuntime()
		}
	case "a":
		if v.toggleGateApproval() {
			return v.syncRuntime()
		}
	}
	return nil
}

func (v *workflowView) ensureRuntime() error {
	if v.moduleCtx == nil {
		v.moduleCtx = module.NewContext(v.app.config, v.app.workflow, v.app.orchestrator, v.app.logbook)
	}
	if v.registry == nil {
		factory := v.registryFactory
		if factory == nil {
			factory = defaultModuleRegistryFactory
		}
		reg, err := factory(v.app.config)
		if err != nil {
			return err
		}
		v.registry = reg
	}
	if v.engine == nil {
		repo := engine.NewRepository(v.app.workflow)
		eng, err := engine.New(v.registry, repo)
		if err != nil {
			return err
		}
		v.engine = eng
	}
	if v.loader == nil {
		v.loader = defaultWorkflowLoader
	}
	return nil
}

func (v *workflowView) bootstrap(resume bool) (engine.State, error) {
	if err := v.ensureRuntime(); err != nil {
		return engine.State{}, err
	}
	if resume {
		state, err := v.engine.Resume(v.moduleCtx, engine.ResumeRequest{Runtime: v.runtimeOverrides()})
		if err != nil {
			if errors.Is(err, engine.ErrStateNotFound) {
				resume = false
			} else {
				return engine.State{}, err
			}
		} else {
			return state, nil
		}
	}
	def := v.definition
	if def.ID == "" {
		loaded, err := v.loader(v.app.config, v.workflowID)
		if err != nil {
			return engine.State{}, err
		}
		def = loaded
	}
	state, err := v.engine.Start(v.moduleCtx, engine.StartRequest{Definition: def})
	if err != nil {
		return engine.State{}, err
	}
	return state, nil
}

func (v *workflowView) runSelectedModule() tea.Cmd {
	if !v.stateLoaded || len(v.state.Nodes) == 0 {
		return nil
	}
	node := v.state.Nodes[v.selection]
	if _, ok := v.running[node.ID]; ok {
		v.setStatus(fmt.Sprintf("%s is already running", node.Name))
		return nil
	}
	if !v.isRunnable(node.ID) {
		v.setStatus(fmt.Sprintf("%s is not ready", node.Name))
		return nil
	}
	return v.claimModules([]string{node.ID})
}

func (v *workflowView) claimModules(ids []string) tea.Cmd {
	if v.engine == nil {
		return nil
	}
	modules := cloneStrings(ids)
	limit := len(modules)
	if limit == 0 {
		limit = 1
	}
	overrides := v.runtimeOverrides()
	return func() tea.Msg {
		result, err := v.engine.Claim(v.moduleCtx, engine.ClaimRequest{
			Runtime: overrides,
			Limit:   limit,
			Modules: modules,
		})
		return workClaimMsg{result: result, err: err}
	}
}

func (v *workflowView) launchClaims(claims []engine.WorkClaim) tea.Cmd {
	if len(claims) == 0 {
		return nil
	}
	cmds := make([]tea.Cmd, 0, len(claims))
	for _, claim := range claims {
		ref, ok := v.moduleRefs[claim.ID]
		if !ok {
			v.setStatus(fmt.Sprintf("Module %s is undefined", claim.ID))
			continue
		}
		mod, err := v.registry.Resolve(ref.ModuleID, convertModuleConfig(ref.Config))
		if err != nil {
			v.setStatus(fmt.Sprintf("Resolve %s: %v", claim.Name, err))
			continue
		}
		cmds = append(cmds, v.executeModule(claim.ID, mod))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (v *workflowView) executeModule(id string, mod module.Module) tea.Cmd {
	ctx := v.moduleCtx.WithMode("workflow-engine")
	return func() tea.Msg {
		result, err := mod.Run(ctx)
		return moduleRunFinishedMsg{id: id, result: result, err: err}
	}
}

func (v *workflowView) handleModuleRunFinished(msg moduleRunFinishedMsg) tea.Cmd {
	if v.engine == nil {
		return nil
	}
	update := engine.ModuleStatusUpdate{
		ID:         msg.id,
		Result:     msg.result,
		Err:        msg.err,
		FinishedAt: time.Now(),
	}
	result := msg.result
	if result.Status == "" {
		if msg.err != nil {
			result.Status = module.StatusFailed
		} else {
			result.Status = module.StatusCompleted
		}
		update.Result = result
	}
	state, err := v.engine.Update(v.moduleCtx, engine.UpdateRequest{
		Runtime: v.runtimeOverrides(),
		Results: []engine.ModuleStatusUpdate{update},
	})
	if err != nil {
		v.setStatus(fmt.Sprintf("Engine update failed: %v", err))
		return nil
	}
	return v.applyState(state)
}

func (v *workflowView) refreshEngineState() tea.Cmd {
	if v.engine == nil || v.finished {
		return nil
	}
	return func() tea.Msg {
		state, err := v.engine.Update(v.moduleCtx, engine.UpdateRequest{Runtime: v.runtimeOverrides()})
		return workflowStateMsg{state: state, err: err}
	}
}

func (v *workflowView) scheduleRefresh() tea.Cmd {
	if v.engine == nil || v.finished {
		return nil
	}
	return tea.Tick(engineRefreshInterval, func(time.Time) tea.Msg {
		return engineRefreshRequest{}
	})
}

func (v *workflowView) skipSelectedModule() bool {
	if len(v.state.Nodes) == 0 {
		return false
	}
	node := v.state.Nodes[v.selection]
	ref, ok := v.moduleRefs[node.ID]
	if !ok || !ref.Optional {
		v.setStatus(fmt.Sprintf("%s cannot be skipped", node.Name))
		return false
	}
	if len(v.targets) == 0 {
		return false
	}
	updated := make([]string, 0, len(v.targets))
	removed := false
	for _, id := range v.targets {
		if id == node.ID {
			removed = true
			continue
		}
		updated = append(updated, id)
	}
	if !removed {
		v.setStatus(fmt.Sprintf("%s already skipped", node.Name))
		return false
	}
	v.targets = updated
	v.setStatus(fmt.Sprintf("Skipped optional module %s", node.Name))
	return true
}

func (v *workflowView) toggleGateRequirement() bool {
	node := v.currentNode()
	if node == nil {
		return false
	}
	gate := v.manualGates[node.ID]
	gate.Required = !gate.Required
	if !gate.Required {
		gate.Approved = false
		gate.Note = ""
	}
	if v.manualGates == nil {
		v.manualGates = map[string]scheduler.ManualGateState{}
	}
	v.manualGates[node.ID] = gate
	if gate.Required {
		v.setStatus(fmt.Sprintf("Manual approval required for %s", node.Name))
	} else {
		v.setStatus(fmt.Sprintf("Manual gate removed for %s", node.Name))
	}
	return true
}

func (v *workflowView) toggleGateApproval() bool {
	node := v.currentNode()
	if node == nil {
		return false
	}
	gate, ok := v.manualGates[node.ID]
	if !ok || !gate.Required {
		v.setStatus("Manual gate not required for this module")
		return false
	}
	gate.Approved = !gate.Approved
	v.manualGates[node.ID] = gate
	if gate.Approved {
		v.setStatus(fmt.Sprintf("Approved %s", node.Name))
	} else {
		v.setStatus(fmt.Sprintf("Approval revoked for %s", node.Name))
	}
	return true
}

func (v *workflowView) currentNode() *engine.ModuleStatus {
	if !v.stateLoaded || len(v.state.Nodes) == 0 {
		return nil
	}
	if v.selection < 0 {
		v.selection = 0
	}
	if v.selection >= len(v.state.Nodes) {
		v.selection = len(v.state.Nodes) - 1
	}
	return &v.state.Nodes[v.selection]
}

func (v *workflowView) installDefinition(def workflow.WorkflowDefinition) {
	if len(def.Modules) == 0 {
		return
	}
	refs := make(map[string]workflow.ModuleRef, len(def.Modules))
	for _, ref := range def.Modules {
		refs[ref.InstanceID()] = ref
	}
	v.definition = def
	v.moduleRefs = refs
	if len(v.targets) == 0 {
		v.targets = def.ModuleIDs()
	}
}

func (v *workflowView) installRuntimeState(state engine.State) {
	v.running = map[string]struct{}{}
	for _, id := range state.Runtime.Running {
		if strings.TrimSpace(id) == "" {
			continue
		}
		v.running[id] = struct{}{}
	}
	if len(state.Runtime.ManualGates) > 0 {
		v.manualGates = cloneManualGates(state.Runtime.ManualGates)
	}
	if len(state.Runtime.Targets) > 0 {
		v.targets = cloneStrings(state.Runtime.Targets)
	} else if len(v.targets) == 0 && len(state.Definition.Modules) > 0 {
		v.targets = state.Definition.ModuleIDs()
	}
	if v.selection >= len(state.Nodes) {
		v.selection = max(0, len(state.Nodes)-1)
	}
}

func (v *workflowView) applyState(state engine.State) tea.Cmd {
	v.state = state
	v.installDefinition(state.Definition)
	v.installRuntimeState(state)
	return v.checkForCompletion()
}

func (v *workflowView) checkForCompletion() tea.Cmd {
	if v.finished {
		return nil
	}
	if v.state.Status == engine.EngineStatusComplete {
		return v.workflowFinished("engine-complete")
	}
	if len(v.state.Nodes) == 0 {
		return v.workflowFinished("no-modules")
	}
	return nil
}

func (v *workflowView) workflowFinished(reason string) tea.Cmd {
	if v.finished {
		return nil
	}
	v.finished = true
	status := strings.TrimSpace(string(v.state.Status))
	if status == "" {
		status = "complete"
	}
	v.setStatus(fmt.Sprintf("Workflow finished (%s) · returning to menu", friendlyLabel(status)))
	msg := workflowFinishedMsg{
		WorkflowID: v.state.WorkflowID,
		RunID:      v.state.RunID,
		Status:     v.state.Status,
		Reason:     reason,
	}
	return func() tea.Msg { return msg }
}

func (v *workflowView) runtimeOverrides() *engine.RuntimeOverrides {
	overrides := &engine.RuntimeOverrides{}
	if len(v.targets) > 0 {
		targets := cloneStrings(v.targets)
		overrides.Targets = &targets
	}
	if len(v.manualGates) > 0 {
		gates := cloneManualGates(v.manualGates)
		overrides.ManualGates = &gates
	}
	return overrides
}

func (v *workflowView) syncRuntime() tea.Cmd {
	if v.engine == nil {
		return nil
	}
	return func() tea.Msg {
		state, err := v.engine.Update(v.moduleCtx, engine.UpdateRequest{Runtime: v.runtimeOverrides()})
		return workflowStateMsg{state: state, err: err}
	}
}

func (v *workflowView) isRunnable(id string) bool {
	for _, runnable := range v.state.Runnable {
		if runnable == id {
			return true
		}
	}
	return false
}

func (v *workflowView) setStatus(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	v.app.statusMsg = message
	v.app.logProgress(message)
}

func convertModuleConfig(cfg workflow.ModuleConfig) module.Config {
	if len(cfg) == 0 {
		return nil
	}
	out := make(module.Config, len(cfg))
	for key, value := range cfg {
		out[key] = value
	}
	return out
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	dup := make([]string, len(values))
	copy(dup, values)
	return dup
}

func cloneManualGates(values map[string]scheduler.ManualGateState) map[string]scheduler.ManualGateState {
	if len(values) == 0 {
		return nil
	}
	dup := make(map[string]scheduler.ManualGateState, len(values))
	for id, state := range values {
		dup[id] = state
	}
	return dup
}

func defaultModuleRegistryFactory(cfg *config.Config) (*module.Registry, error) {
	reg := module.NewRegistry()
	modules.RegisterBuiltins(reg)
	if err := plugins.RegisterSkillPlugins(reg, cfg); err != nil {
		return nil, err
	}
	return reg, nil
}

func defaultWorkflowLoader(cfg *config.Config, workflowID string) (workflow.WorkflowDefinition, error) {
	if cfg == nil {
		return workflow.WorkflowDefinition{}, fmt.Errorf("missing project config")
	}
	fileNames := []string{
		fmt.Sprintf("%s.yaml", workflowID),
		fmt.Sprintf("%s.yml", workflowID),
	}
	var candidates []string
	appendPaths := func(base string) {
		if strings.TrimSpace(base) == "" {
			return
		}
		for _, name := range fileNames {
			candidates = append(candidates, filepath.Join(base, "workflows", name))
		}
	}
	appendPaths(cfg.ProjectDir)
	appendPaths(cfg.LatticeProjectDir)
	appendPaths(cfg.LatticeRoot)
	for _, name := range fileNames {
		candidates = append(candidates, filepath.Join(workflow.DefaultWorkflowDir, name))
	}
	visited := map[string]struct{}{}
	for _, path := range candidates {
		clean := filepath.Clean(path)
		if _, seen := visited[clean]; seen {
			continue
		}
		visited[clean] = struct{}{}
		if info, err := os.Stat(clean); err == nil && !info.IsDir() {
			return workflow.LoadDefinitionFile(clean)
		}
	}
	return workflow.WorkflowDefinition{}, fmt.Errorf("workflow definition %s not found", workflowID)
}
