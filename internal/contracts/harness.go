package contracts

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Report captures validation results for an agent file.
type Report struct {
	Path   string
	Role   string
	Errors []error
}

// ValidateAgentFile reads and validates an agent.yaml file.
func ValidateAgentFile(path string) (*Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read agent file: %w", err)
	}
	var spec AgentSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse agent file: %w", err)
	}
	report := &Report{
		Path:   path,
		Role:   spec.Lattice.Role,
		Errors: ValidateAgent(&spec),
	}
	return report, nil
}

// IsValid reports whether the validation passed.
func (r *Report) IsValid() bool {
	return r != nil && len(r.Errors) == 0
}
