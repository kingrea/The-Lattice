# Plugin Modules

Lattice can extend the workflow engine with project-defined modules located
under `.lattice/modules/`. Each module is declared as either a YAML file
describing a skill runner or a Go file that returns the same schema
programmatically. At runtime the CLI loads built-ins first and then registers
every custom definition it finds.

## Directory Layout

```
.lattice/
└── modules/
    ├── custom-module.yaml    # YAML definition
    └── generate-modules.go   # Go definitions
```

Only files directly inside `.lattice/modules/` are scanned. `.yaml` / `.yml`
files are interpreted as declarative skill modules. `.go` files are executed
through yaegi and must expose a `ModuleDefinitions() ([]map[string]any, error)`
function that returns the same structure as a YAML document.

## YAML Schema

```yaml
id: custom-docs # required, must be unique
name: "Prepare Custom Docs" # optional display name (defaults to id)
description: >- # optional summary shown in the TUI
  Guides an operator through bespoke documentation.
version: 1.0.0 # required, used for artifact metadata
concurrency: # optional scheduling profile
  slots: 1
  exclusive: false
skill:
  slug: lattice-planning # bundled skill slug, or provide `path`
  # path: ./skills/custom/SKILL.md
  prompt: |
    Load {{ .SkillPath }} and capture every decision inside {{ .WorkflowDir }}/plan/CUSTOM.md.
    Inputs: {{ range $i, $ref := .Inputs }}{{ if $i }}, {{ end }}{{ $ref.ID }}{{ end }}
  window_name: custom-docs # optional fixed tmux window name
  env: # optional env vars exported before opencode executes
    COMMISSION_PATH: "{{ .WorkflowDir }}/plan/COMMISSION.md"
  variables: # free-form template key/value pairs
    reviewer: Dr. Rivera
inputs: # optional references to existing artifacts
  - artifact: modules-doc
outputs: # at least one output is required
  - artifact: commission-doc
    optional: false
config: # optional default module config, merged with workflow overrides
  reviewer: dr-rivera
```

### Prompt Template Data

`skill.prompt` is rendered with the Go `text/template` engine. The template has
access to:

| Key           | Description                                          |
| ------------- | ---------------------------------------------------- |
| `SkillPath`   | Absolute path to the skill file loaded into OpenCode |
| `Definition`  | Full `ModuleDefinition` struct                       |
| `Inputs`      | Slice of resolved `artifact.ArtifactRef` values      |
| `Outputs`     | Slice of resolved `artifact.ArtifactRef` values      |
| `Config`      | Definition defaults merged with workflow overrides   |
| `Variables`   | `skill.variables` map for ad-hoc values              |
| `ProjectDir`  | Repository root                                      |
| `WorkflowDir` | `.lattice/workflow` directory for the project        |
| `SkillsDir`   | Project-local skills directory                       |

The template function map currently includes `join` (from `strings.Join`) for
string slices; for struct values iterate manually as shown above.

### Metadata + Completion

Skill modules launch OpenCode inside a tmux window. Completion is determined by
checking the declared output artifacts:

- Markers/directories are considered ready once they exist.
- Documents/JSON files are checked for `_lattice` metadata. If the metadata was
  produced by a different module or version, the runner rewrites it using the
  plugin's `id` and `version`.
- When every output is ready the tmux session is closed automatically.

## Go Definitions

Go files provide a light abstraction over YAML—write Go when you want to
generate definitions dynamically. Each file must define:

```go
package main

func ModuleDefinitions() ([]map[string]any, error) {
    return []map[string]any{
        {
            "id":      "go-defined",
            "version": "1.0.0",
            "skill": map[string]any{
                "slug":   "create-agent-file",
                "prompt": "Load {{ .SkillPath }} and summarize the agent dossier.",
            },
            "outputs": []map[string]any{{{"artifact": "commission-doc"}}},
        },
    }, nil
}
```

`[]map[string]any` uses the same shape as a YAML document, so you can mix
literal maps with helpers, loops, or other Go functions. Returning an error
aborts plugin loading so the failure is visible in the CLI.

## Sample Definitions

See `docs/examples/modules/custom-docs.yaml` for a complete YAML definition that
renders a custom prompt and sets env vars. Copy it into `.lattice/modules/` to
try the plugin locally.
