package module

import (
	"strings"

	"github.com/kingrea/The-Lattice/internal/artifact"
)

// Fingerprinter can be implemented by modules that expose deterministic
// fingerprints for their output artifacts. The resolver/runtime uses these
// values to detect stale artifacts without invoking the module.
type Fingerprinter interface {
	ArtifactFingerprints(ctx *ModuleContext) (map[string]string, error)
}

// ArtifactStatus captures the readiness/freshness of an artifact from the
// resolver's perspective.
type ArtifactStatus string

const (
	ArtifactStatusUnknown  ArtifactStatus = "unknown"
	ArtifactStatusFresh    ArtifactStatus = "fresh"
	ArtifactStatusReady    ArtifactStatus = "ready"
	ArtifactStatusMissing  ArtifactStatus = "missing"
	ArtifactStatusInvalid  ArtifactStatus = "invalid"
	ArtifactStatusOutdated ArtifactStatus = "outdated"
	ArtifactStatusError    ArtifactStatus = "error"
)

// ArtifactInvalidationReason enumerates why an artifact was considered stale.
type ArtifactInvalidationReason string

const (
	InvalidationReasonMissing         ArtifactInvalidationReason = "missing"
	InvalidationReasonInvalidMetadata ArtifactInvalidationReason = "invalid-metadata"
	InvalidationReasonVersionMismatch ArtifactInvalidationReason = "version-mismatch"
	InvalidationReasonFingerprint     ArtifactInvalidationReason = "fingerprint-mismatch"
	InvalidationReasonCheckError      ArtifactInvalidationReason = "check-error"
)

// ArtifactInvalidation is emitted when Resolver.CheckArtifact determines an
// output is stale or invalid. Implement ArtifactInvalidationHandler to respond
// to these notifications (e.g., enqueue reruns, clean up derived artifacts).
type ArtifactInvalidation struct {
	Artifact            artifact.ArtifactRef
	Status              ArtifactStatus
	Reason              ArtifactInvalidationReason
	StoredFingerprint   string
	ExpectedFingerprint string
	Metadata            *artifact.Metadata
	Err                 error
}

// ArtifactInvalidationHandler allows modules to react to stale artifacts.
type ArtifactInvalidationHandler interface {
	OnArtifactInvalidation(ctx *ModuleContext, event ArtifactInvalidation) error
}

const fingerprintNotePrefix = "fingerprint:"

// FingerprintNoteKey returns the metadata note key for an artifact fingerprint.
func FingerprintNoteKey(artifactID string) string {
	id := strings.TrimSpace(artifactID)
	if id == "" {
		return fingerprintNotePrefix + "default"
	}
	return fingerprintNotePrefix + id
}
