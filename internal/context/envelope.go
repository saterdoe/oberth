package context

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const ContextEnvelopeSchemaVersion = "1"

// ContextEnvelopeV1 is the immutable source-of-truth shared by every stage in
// one task run. Stage-specific prompts may select less content, but must retain
// this identity and its repository and requirement constraints.
type ContextEnvelopeV1 struct {
	SchemaVersion string                   `json:"schema_version"`
	Fingerprint   string                   `json:"fingerprint"`
	Task          ContextEnvelopeTaskV1    `json:"task"`
	Repository    ContextEnvelopeRepoV1    `json:"repository"`
	Compilation   ContextEnvelopeCompileV1 `json:"compilation"`
}

type ContextEnvelopeTaskV1 struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Type        string          `json:"type,omitempty"`
	Constraints json.RawMessage `json:"constraints,omitempty"`
}

type ContextEnvelopeRepoV1 struct {
	Identity   string `json:"identity"`
	BaseCommit string `json:"base_commit"`
}

type ContextEnvelopeCompileV1 struct {
	Sources    []SourceSelection  `json:"sources"`
	Exclusions []ContextExclusion `json:"exclusions,omitempty"`
	Metrics    CompileMetrics     `json:"metrics"`
	Retrieval  RetrievalMetrics   `json:"retrieval"`
}

// NewContextEnvelopeV1 constructs and fingerprints a deterministic envelope.
// Volatile values such as timestamps are deliberately excluded so restart and
// recovery can reproduce the same identity from the same inputs.
func NewContextEnvelopeV1(task ContextEnvelopeTaskV1, repository ContextEnvelopeRepoV1, compiled *CompileResult, retrieval RetrievalMetrics) (ContextEnvelopeV1, error) {
	if compiled == nil {
		return ContextEnvelopeV1{}, errors.New("compiled context is required")
	}
	envelope := ContextEnvelopeV1{
		SchemaVersion: ContextEnvelopeSchemaVersion,
		Task:          task,
		Repository:    repository,
		Compilation: ContextEnvelopeCompileV1{
			Sources:    append([]SourceSelection(nil), compiled.Manifest...),
			Exclusions: append([]ContextExclusion(nil), compiled.Exclusions...),
			Metrics:    compiled.Metrics,
			Retrieval:  retrieval,
		},
	}
	if err := envelope.Validate(); err != nil {
		return ContextEnvelopeV1{}, err
	}
	fingerprint, err := envelope.calculateFingerprint()
	if err != nil {
		return ContextEnvelopeV1{}, err
	}
	envelope.Fingerprint = fingerprint
	return envelope, nil
}

func (e ContextEnvelopeV1) Validate() error {
	if e.SchemaVersion != ContextEnvelopeSchemaVersion {
		return fmt.Errorf("unsupported context envelope schema %q", e.SchemaVersion)
	}
	if strings.TrimSpace(e.Task.ID) == "" {
		return errors.New("context envelope task id is required")
	}
	if strings.TrimSpace(e.Repository.Identity) == "" {
		return errors.New("context envelope repository identity is required")
	}
	if strings.TrimSpace(e.Repository.BaseCommit) == "" {
		return errors.New("context envelope base commit is required")
	}
	return nil
}

func (e ContextEnvelopeV1) calculateFingerprint() (string, error) {
	e.Fingerprint = ""
	encoded, err := json.Marshal(e)
	if err != nil {
		return "", fmt.Errorf("marshal context envelope: %w", err)
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(encoded)), nil
}

// VerifyFingerprint detects accidental or unsafe mutation after compilation.
func (e ContextEnvelopeV1) VerifyFingerprint() error {
	if err := e.Validate(); err != nil {
		return err
	}
	want, err := e.calculateFingerprint()
	if err != nil {
		return err
	}
	if e.Fingerprint != want {
		return fmt.Errorf("context envelope fingerprint mismatch: got %q want %q", e.Fingerprint, want)
	}
	return nil
}
