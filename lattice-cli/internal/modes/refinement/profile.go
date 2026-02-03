package refinement

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type projectProfile struct {
	Type string
	Tags []string
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
	if profile.Type == "general" {
		if fileExists(filepath.Join(root, "Cargo.toml")) {
			profile.Type = "rust-app"
			profile.Tags = mergeTags(profile.Tags, []string{"rust"})
		}
	}
	profile.Tags = dedupe(profile.Tags)
	if len(profile.Tags) == 0 {
		profile.Tags = []string{"generalist"}
	}
	return profile
}

func (p projectProfile) Summary() string {
	label := strings.ReplaceAll(p.Type, "-", " ")
	if len(p.Tags) == 0 {
		return label
	}
	return label + " (" + strings.Join(p.Tags, ", ") + ")"
}

type packageManifest struct {
	Name             string            `json:"name"`
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
