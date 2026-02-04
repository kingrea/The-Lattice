package module

import "github.com/yourusername/lattice/internal/artifact"

// Base provides common plumbing for modules (identity + IO contracts).
type Base struct {
	info    Info
	inputs  []artifact.ArtifactRef
	outputs []artifact.ArtifactRef
}

// NewBase seeds the helper with module info.
func NewBase(info Info) Base {
	return Base{info: info}
}

// SetInputs declares the required artifacts.
func (b *Base) SetInputs(refs ...artifact.ArtifactRef) {
	b.inputs = append([]artifact.ArtifactRef{}, refs...)
}

// SetOutputs declares the produced artifacts.
func (b *Base) SetOutputs(refs ...artifact.ArtifactRef) {
	b.outputs = append([]artifact.ArtifactRef{}, refs...)
}

// Info implements Module.Info.
func (b *Base) Info() Info {
	return b.info
}

// Inputs implements Module.Inputs.
func (b *Base) Inputs() []artifact.ArtifactRef {
	return append([]artifact.ArtifactRef{}, b.inputs...)
}

// Outputs implements Module.Outputs.
func (b *Base) Outputs() []artifact.ArtifactRef {
	return append([]artifact.ArtifactRef{}, b.outputs...)
}
