package contracts

import (
	"fmt"
)

// AgentSpec defines the expected agent.yaml schema.
type AgentSpec struct {
	Lattice     LatticeMeta `yaml:"lattice"`
	Name        string      `yaml:"name"`
	Description string      `yaml:"description"`
	Community   string      `yaml:"community"`
	Model       string      `yaml:"model"`
	Color       string      `yaml:"color"`
	Skills      []SkillRef  `yaml:"skills"`
	Extensions  []SkillRef  `yaml:"extensions"`
}

// LatticeMeta captures shared metadata for lattice files.
type LatticeMeta struct {
	Type    string `yaml:"type"`
	Role    string `yaml:"role"`
	Version int    `yaml:"version"`
}

// SkillRef points to a skill definition.
type SkillRef struct {
	ID   string `yaml:"id"`
	Path string `yaml:"path"`
}

// ValidateAgent checks the agent spec against core contract expectations.
func ValidateAgent(spec *AgentSpec) []error {
	var errs []error
	if spec == nil {
		return []error{fmt.Errorf("agent spec is nil")}
	}
	if spec.Lattice.Type != "core-agent" {
		errs = append(errs, fmt.Errorf("lattice.type must be core-agent"))
	}
	if spec.Lattice.Version != 1 {
		errs = append(errs, fmt.Errorf("lattice.version must be 1"))
	}
	if spec.Lattice.Role == "" {
		errs = append(errs, fmt.Errorf("lattice.role is required"))
	}
	if spec.Name == "" {
		errs = append(errs, fmt.Errorf("name is required"))
	}
	if spec.Description == "" {
		errs = append(errs, fmt.Errorf("description is required"))
	}
	if spec.Community == "" {
		errs = append(errs, fmt.Errorf("community is required"))
	}
	if spec.Model == "" {
		errs = append(errs, fmt.Errorf("model is required"))
	}
	if len(spec.Skills) == 0 {
		errs = append(errs, fmt.Errorf("skills list is required"))
	}

	seenSkillIDs := map[string]struct{}{}
	for index, skill := range spec.Skills {
		if skill.ID == "" {
			errs = append(errs, fmt.Errorf("skills[%d].id is required", index))
		} else {
			if _, exists := seenSkillIDs[skill.ID]; exists {
				errs = append(errs, fmt.Errorf("skills[%d].id duplicates %q", index, skill.ID))
			}
			seenSkillIDs[skill.ID] = struct{}{}
		}
		if skill.Path == "" {
			errs = append(errs, fmt.Errorf("skills[%d].path is required", index))
		}
	}

	for index, skill := range spec.Extensions {
		if skill.ID == "" {
			errs = append(errs, fmt.Errorf("extensions[%d].id is required", index))
		}
		if skill.Path == "" {
			errs = append(errs, fmt.Errorf("extensions[%d].path is required", index))
		}
	}

	if contract, ok := ContractForRole(spec.Lattice.Role); ok {
		for _, required := range contract.RequiredSkills {
			if _, ok := seenSkillIDs[required]; !ok {
				errs = append(errs, fmt.Errorf("missing required skill %q", required))
			}
		}
	} else if spec.Lattice.Role != "" {
		errs = append(errs, fmt.Errorf("unknown lattice.role %q", spec.Lattice.Role))
	}

	return errs
}
