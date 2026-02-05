package refinement

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kingrea/The-Lattice/internal/artifact"
	"github.com/kingrea/The-Lattice/internal/module"
	"github.com/kingrea/The-Lattice/internal/modules/runtime"
	"github.com/kingrea/The-Lattice/internal/orchestrator"
)

type stakeholderAssignment struct {
	Role   string
	Agent  orchestrator.ProjectAgent
	Reused bool
	Repeat bool
}

func planStakeholderAssignments(ctx *module.ModuleContext, client orchestratorClient, profile projectProfile) ([]stakeholderAssignment, error) {
	agents, err := client.LoadProjectAgents()
	if err != nil {
		return nil, fmt.Errorf("%s: load project agents: %w", moduleID, err)
	}
	if len(agents) == 0 {
		return nil, fmt.Errorf("%s: no project agents available for refinement", moduleID)
	}
	workerList := client.CurrentWorkerList()
	used := make(map[string]struct{})
	if workerList.Orchestrator != nil {
		if name := strings.ToLower(strings.TrimSpace(workerList.Orchestrator.Name)); name != "" {
			used[name] = struct{}{}
		}
	}
	for _, worker := range workerList.Workers {
		if name := strings.ToLower(strings.TrimSpace(worker.Name)); name != "" {
			used[name] = struct{}{}
		}
	}
	roles := generateRoles(profile)
	assignments, err := assignRoles(roles, agents, used)
	if err != nil {
		return nil, err
	}
	return assignments, nil
}

func (m *Module) writeStakeholdersManifest(ctx *module.ModuleContext, assignments []stakeholderAssignment, profile projectProfile) error {
	payload := struct {
		ProjectType string                    `json:"projectType"`
		Tags        []string                  `json:"tags,omitempty"`
		GeneratedAt string                    `json:"generatedAt"`
		Roles       map[string]map[string]any `json:"roles"`
	}{
		ProjectType: profile.Type,
		Tags:        profile.Tags,
		GeneratedAt: m.now().UTC().Format(time.RFC3339),
		Roles:       make(map[string]map[string]any, len(assignments)),
	}
	for _, assignment := range assignments {
		record := map[string]any{
			"name": assignment.Agent.Name,
		}
		if assignment.Agent.Path != "" {
			record["agentPath"] = assignment.Agent.Path
		}
		if assignment.Agent.Memory != "" {
			record["memoryPath"] = assignment.Agent.Memory
		}
		if assignment.Reused {
			record["reused"] = true
		}
		if assignment.Repeat {
			record["repeat"] = true
		}
		payload.Roles[assignment.Role] = record
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: encode stakeholders manifest: %w", moduleID, err)
	}
	meta := m.metadataFor(ctx, artifact.StakeholdersJSON, artifact.RefinementNeededMarker)
	runtime.WithFingerprint(artifact.StakeholdersJSON, stakeholderFingerprint(assignments, profile))(&meta)
	if err := ctx.Artifacts.Write(artifact.StakeholdersJSON, data, meta); err != nil {
		return fmt.Errorf("%s: write stakeholders manifest: %w", moduleID, err)
	}
	return nil
}

func stakeholderFingerprint(assignments []stakeholderAssignment, profile projectProfile) string {
	parts := []string{strings.ToLower(strings.TrimSpace(profile.Type)), strings.Join(profile.Tags, ",")}
	sorted := append([]stakeholderAssignment(nil), assignments...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return strings.ToLower(sorted[i].Role) < strings.ToLower(sorted[j].Role)
	})
	for _, assignment := range sorted {
		parts = append(parts, fmt.Sprintf("%s|%s|%t|%t", strings.ToLower(strings.TrimSpace(assignment.Role)), strings.ToLower(strings.TrimSpace(assignment.Agent.Name)), assignment.Reused, assignment.Repeat))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, ";")))
	return fmt.Sprintf("%x", sum[:])
}

func assignRoles(roles []string, agents []orchestrator.ProjectAgent, used map[string]struct{}) ([]stakeholderAssignment, error) {
	var unused, returning []orchestrator.ProjectAgent
	for _, agent := range agents {
		key := strings.ToLower(strings.TrimSpace(agent.Name))
		if key == "" {
			continue
		}
		if _, ok := used[key]; ok {
			returning = append(returning, agent)
			continue
		}
		unused = append(unused, agent)
	}
	candidates := append([]orchestrator.ProjectAgent{}, unused...)
	candidates = append(candidates, returning...)
	if len(candidates) == 0 {
		candidates = append(candidates, returning...)
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("%s: no candidates available for stakeholder roles", moduleID)
	}
	assignCount := make(map[string]int)
	assignments := make([]stakeholderAssignment, 0, len(roles))
	for i, role := range roles {
		agent := candidates[i%len(candidates)]
		key := strings.ToLower(strings.TrimSpace(agent.Name))
		assignCount[key]++
		_, reused := used[key]
		assignments = append(assignments, stakeholderAssignment{
			Role:   role,
			Agent:  agent,
			Reused: reused,
			Repeat: assignCount[key] > 1,
		})
	}
	sort.SliceStable(assignments, func(i, j int) bool { return assignments[i].Role < assignments[j].Role })
	return assignments, nil
}

type projectProfile struct {
	Type string
	Tags []string
}

func (p projectProfile) Summary() string {
	label := strings.ReplaceAll(p.Type, "-", " ")
	if len(p.Tags) == 0 {
		return label
	}
	return label + " (" + strings.Join(p.Tags, ", ") + ")"
}

func detectProjectProfile(root string) projectProfile {
	profile := projectProfile{Type: "general", Tags: []string{"generalist"}}
	manifest := loadPackageManifest(root)
	if manifest != nil {
		if kind := classifyJSManifest(manifest); kind != "" {
			profile.Type = kind
		}
		profile.Tags = mergeTags(profile.Tags, manifest.Tags())
	}
	if profile.Type == "general" {
		if fileExists(filepath.Join(root, "go.mod")) {
			profile.Type = "go-service"
			profile.Tags = mergeTags(profile.Tags, []string{"go", "backend"})
		} else if fileExists(filepath.Join(root, "pyproject.toml")) || fileExists(filepath.Join(root, "requirements.txt")) {
			profile.Type = "python-app"
			profile.Tags = mergeTags(profile.Tags, []string{"python"})
		}
	}
	if profile.Type == "general" && fileExists(filepath.Join(root, "Cargo.toml")) {
		profile.Type = "rust-app"
		profile.Tags = mergeTags(profile.Tags, []string{"rust"})
	}
	profile.Tags = dedupe(profile.Tags)
	if len(profile.Tags) == 0 {
		profile.Tags = []string{"generalist"}
	}
	return profile
}

type packageManifest struct {
	Dependencies     map[string]string `json:"dependencies"`
	DevDependencies  map[string]string `json:"devDependencies"`
	PeerDependencies map[string]string `json:"peerDependencies"`
}

func (m *packageManifest) Tags() []string {
	var tags []string
	tags = append(tags, mapKeys(m.Dependencies)...)
	tags = append(tags, mapKeys(m.DevDependencies)...)
	tags = append(tags, mapKeys(m.PeerDependencies)...)
	return tags
}

func loadPackageManifest(root string) *packageManifest {
	path := filepath.Join(root, "package.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var manifest packageManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil
	}
	return &manifest
}

func classifyJSManifest(m *packageManifest) string {
	tags := strings.Join(append(mapKeys(m.Dependencies), mapKeys(m.DevDependencies)...), ",")
	lower := strings.ToLower(tags)
	switch {
	case strings.Contains(lower, "vue") || strings.Contains(lower, "nuxt"):
		return "frontend-vue"
	case strings.Contains(lower, "react") || strings.Contains(lower, "next") || strings.Contains(lower, "remix"):
		return "frontend-react"
	case strings.Contains(lower, "svelte"):
		return "frontend-svelte"
	case strings.Contains(lower, "angular"):
		return "frontend-angular"
	case strings.Contains(lower, "electron"):
		return "desktop-electron"
	case containsAny(lower, []string{"express", "fastify", "koa", "nest"}):
		return "node-service"
	default:
		if lower != "" {
			return "node-app"
		}
	}
	return ""
}

func containsAny(haystack string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

var roleTemplates = map[string][]string{
	"frontend-vue": {
		"Vue Staff Engineer",
		"Design Systems Architect",
		"Frontend QA Specialist",
		"Performance Optimization Engineer",
		"Accessibility Lead",
		"Browser Compatibility Analyst",
		"Animations & Motion Reviewer",
		"Component Library Steward",
		"DX Coach",
		"DevOps Release Captain",
	},
	"frontend-react": {
		"React Staff Engineer",
		"Design Systems Architect",
		"State Management Reviewer",
		"Accessibility Champion",
		"SSR Performance Engineer",
		"QA Automation Lead",
		"Security Analyst",
		"UX Research Liaison",
		"Internationalization Reviewer",
		"Bundler Optimization Lead",
	},
	"frontend-svelte": {
		"Svelte Principal Engineer",
		"Compiler Performance Analyst",
		"Styling Consistency Reviewer",
		"Accessibility Lead",
		"Form Validation Expert",
		"QA Automation Lead",
		"DX Coach",
		"Animation Curator",
		"Internationalization Reviewer",
		"DevOps Captain",
	},
	"frontend-angular": {
		"Angular Architect",
		"Type Safety Reviewer",
		"Change Detection Specialist",
		"Accessibility Advocate",
		"Security Champion",
		"Testing & QA Lead",
		"Documentation Steward",
		"Performance Engineer",
		"Release Captain",
		"Observability Reviewer",
	},
	"node-service": {
		"Backend Staff Engineer",
		"API Contract Reviewer",
		"Database Reliability Lead",
		"Security Engineer",
		"Load Testing Lead",
		"Observability Advocate",
		"Incident Response Captain",
		"DevOps Release Lead",
		"Cost Optimization Analyst",
		"Integration QA Specialist",
	},
	"node-app": {
		"Node Platform Lead",
		"Full-stack QA Specialist",
		"Security Champion",
		"Performance Engineer",
		"API Contract Reviewer",
		"Documentation Steward",
		"Accessibility Advocate",
		"DevOps Captain",
		"Observability Lead",
		"Customer Support Liaison",
	},
	"go-service": {
		"Go Staff Engineer",
		"API Contract Reviewer",
		"Database Reliability Lead",
		"Security Engineer",
		"Load Testing Lead",
		"Observability Advocate",
		"Resilience Engineer",
		"Deployment Captain",
		"Cost Optimization Lead",
		"Integration QA",
	},
	"python-app": {
		"Python Staff Engineer",
		"Data Integrity Reviewer",
		"API Contract Lead",
		"Security Engineer",
		"QA Automation Lead",
		"Performance Tuning Specialist",
		"DevOps Release Captain",
		"Documentation Steward",
		"Customer Advocate",
		"Observability Lead",
	},
	"rust-app": {
		"Rust Systems Architect",
		"Memory Safety Reviewer",
		"Performance Engineer",
		"Security Analyst",
		"Testing & QA Lead",
		"DevOps Captain",
		"Tooling & DX Coach",
		"Documentation Steward",
		"Release Captain",
		"Platform Reliability Lead",
	},
}

var fallbackRoles = []string{
	"Staff Engineer",
	"QA Specialist",
	"Performance Engineer",
	"Security Analyst",
	"Customer Advocate",
	"Documentation Steward",
	"Observability Lead",
	"Integration Tester",
	"Release Captain",
	"Support Liaison",
	"UX Reviewer",
	"Accessibility Champion",
}

func generateRoles(profile projectProfile) []string {
	roles := append([]string{}, roleTemplates[profile.Type]...)
	if len(roles) < 10 {
		roles = append(roles, fallbackRoles...)
	}
	unique := dedupe(roles)
	if len(unique) < 10 {
		for i := len(unique); i < 10; i++ {
			unique = append(unique, fallbackRoles[i%len(fallbackRoles)])
		}
	}
	return unique[:10]
}

func slugify(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return "role"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range lower {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '-' || r == '_':
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "role"
	}
	return result
}

func mergeTags(dst, src []string) []string {
	return dedupe(append(dst, src...))
}

func dedupe(items []string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func mapKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}
