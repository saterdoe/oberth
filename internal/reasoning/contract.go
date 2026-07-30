package reasoning

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const SchemaVersion = "1"

type Kind string

const (
	KindFact       Kind = "fact"
	KindHypothesis Kind = "hypothesis"
	KindAssumption Kind = "assumption"
	KindUnknown    Kind = "unknown"
	KindProperty   Kind = "property"
	KindDecision   Kind = "decision"
)

type Status string

const (
	StatusOpen       Status = "open"
	StatusSupported  Status = "supported"
	StatusRefuted    Status = "refuted"
	StatusUnresolved Status = "unresolved"
	StatusPassed     Status = "passed"
	StatusFailed     Status = "failed"
	StatusUnknown    Status = "unknown"
)

type Record struct {
	ID          string   `json:"id"`
	Kind        Kind     `json:"kind"`
	Statement   string   `json:"statement"`
	Status      Status   `json:"status"`
	Confidence  *float64 `json:"confidence,omitempty"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
	Falsifier   string   `json:"falsifier,omitempty"`
	Scope       string   `json:"scope,omitempty"`
	Required    bool     `json:"required,omitempty"`
	NextAction  string   `json:"next_action,omitempty"`
}

type EvidenceRef struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	Hash        string `json:"hash,omitempty"`
	Subject     string `json:"subject,omitempty"`
	SubjectHash string `json:"subject_hash,omitempty"`
	Detail      string `json:"detail,omitempty"`
	Stale       bool   `json:"stale,omitempty"`
}

type Experiment struct {
	ID            string   `json:"id"`
	Question      string   `json:"question"`
	Preconditions []string `json:"preconditions,omitempty"`
	Environment   string   `json:"environment"`
	Command       string   `json:"command"`
	Expectation   string   `json:"expectation"`
	Observation   string   `json:"observation"`
	Status        Status   `json:"status"`
	DurationMS    int64    `json:"duration_ms,omitempty"`
	Cost          float64  `json:"cost,omitempty"`
	EvidenceIDs   []string `json:"evidence_ids"`
	ClaimIDs      []string `json:"claim_ids,omitempty"`
	Baseline      string   `json:"baseline_fingerprint,omitempty"`
	Candidate     string   `json:"candidate_fingerprint,omitempty"`
}

type Assessment struct {
	MaterialRecords  int      `json:"material_records"`
	SupportedRecords int      `json:"supported_records"`
	CoveragePercent  float64  `json:"coverage_percent"`
	MissingEvidence  []string `json:"missing_evidence"`
	DanglingEvidence []string `json:"dangling_evidence"`
	GateBlockers     []string `json:"gate_blockers"`
}

type CaseV1 struct {
	SchemaVersion string        `json:"schema_version"`
	Records       []Record      `json:"records"`
	Evidence      []EvidenceRef `json:"evidence"`
	Experiments   []Experiment  `json:"experiments"`
	Assessment    Assessment    `json:"assessment"`
}

type ActionArguments struct {
	Record     *Record      `json:"record,omitempty"`
	Evidence   *EvidenceRef `json:"evidence,omitempty"`
	Experiment *Experiment  `json:"experiment,omitempty"`
}

func ParseActionArguments(raw json.RawMessage) (ActionArguments, error) {
	var args ActionArguments
	if err := json.Unmarshal(raw, &args); err != nil {
		return ActionArguments{}, fmt.Errorf("invalid reasoning record: %w", err)
	}
	variants := 0
	if args.Record != nil {
		variants++
	}
	if args.Evidence != nil {
		variants++
	}
	if args.Experiment != nil {
		variants++
	}
	if variants != 1 {
		return ActionArguments{}, errors.New("record_reasoning requires exactly one of record, evidence or experiment")
	}
	if args.Record != nil {
		if err := ValidateRecord(*args.Record); err != nil {
			return ActionArguments{}, err
		}
	}
	if args.Evidence != nil {
		if err := ValidateEvidence(*args.Evidence); err != nil {
			return ActionArguments{}, err
		}
	}
	if args.Experiment != nil {
		if err := ValidateExperiment(*args.Experiment); err != nil {
			return ActionArguments{}, err
		}
	}
	return args, nil
}

func ValidateRecord(record Record) error {
	if strings.TrimSpace(record.ID) == "" || strings.TrimSpace(record.Statement) == "" {
		return errors.New("reasoning record requires id and statement")
	}
	switch record.Kind {
	case KindFact, KindHypothesis, KindAssumption, KindUnknown, KindProperty, KindDecision:
	default:
		return fmt.Errorf("unsupported reasoning kind %q", record.Kind)
	}
	switch record.Status {
	case StatusOpen, StatusSupported, StatusRefuted, StatusUnresolved, StatusPassed, StatusFailed, StatusUnknown:
	default:
		return fmt.Errorf("unsupported reasoning status %q", record.Status)
	}
	if record.Confidence != nil && (*record.Confidence < 0 || *record.Confidence > 1) {
		return errors.New("reasoning confidence must be between 0 and 1")
	}
	if record.Kind == KindUnknown && strings.TrimSpace(record.NextAction) == "" {
		return errors.New("unknown reasoning record requires next_action")
	}
	if record.Kind == KindHypothesis && strings.TrimSpace(record.Falsifier) == "" {
		return errors.New("hypothesis reasoning record requires falsifier")
	}
	if record.Kind == KindProperty && record.Status != StatusPassed && record.Status != StatusFailed && record.Status != StatusUnknown {
		return errors.New("property status must be passed, failed or unknown")
	}
	if isMaterial(record) && (record.Status == StatusSupported || record.Status == StatusPassed) && len(record.EvidenceIDs) == 0 {
		return errors.New("supported reasoning record requires at least one evidence_id")
	}
	return nil
}

func ValidateEvidence(evidence EvidenceRef) error {
	if strings.TrimSpace(evidence.ID) == "" || strings.TrimSpace(evidence.Source) == "" {
		return errors.New("reasoning evidence requires id and source")
	}
	if evidence.Hash != "" && !sha256Pattern.MatchString(evidence.Hash) {
		return errors.New("reasoning evidence hash must be sha256:<64 lowercase hex characters>")
	}
	if evidence.SubjectHash != "" && !sha256Pattern.MatchString(evidence.SubjectHash) {
		return errors.New("reasoning evidence subject_hash must be sha256:<64 lowercase hex characters>")
	}
	return nil
}

func ValidateExperiment(experiment Experiment) error {
	if strings.TrimSpace(experiment.ID) == "" || strings.TrimSpace(experiment.Question) == "" ||
		strings.TrimSpace(experiment.Environment) == "" || strings.TrimSpace(experiment.Command) == "" ||
		strings.TrimSpace(experiment.Expectation) == "" || strings.TrimSpace(experiment.Observation) == "" {
		return errors.New("reasoning experiment requires id, question, environment, command, expectation and observation")
	}
	if experiment.Status != StatusPassed && experiment.Status != StatusFailed && experiment.Status != StatusUnknown {
		return errors.New("reasoning experiment status must be passed, failed or unknown")
	}
	if len(experiment.EvidenceIDs) == 0 {
		return errors.New("reasoning experiment requires at least one evidence_id")
	}
	if experiment.DurationMS < 0 || experiment.Cost < 0 {
		return errors.New("reasoning experiment duration and cost cannot be negative")
	}
	if (experiment.Baseline == "") != (experiment.Candidate == "") {
		return errors.New("reasoning experiment comparison requires both baseline and candidate fingerprints")
	}
	return nil
}

var sha256Pattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func Collect(observations []Observation) *CaseV1 {
	result := &CaseV1{SchemaVersion: SchemaVersion, Records: []Record{}, Evidence: []EvidenceRef{}, Experiments: []Experiment{}}
	recordIDs := map[string]bool{}
	evidenceIDs := map[string]bool{}
	experimentIDs := map[string]bool{}
	for _, observation := range observations {
		if observation.Tool != "record_reasoning" || observation.Status != "ok" {
			if observation.Status == "ok" && observation.Evidence != nil && !evidenceIDs[observation.Evidence.ID] {
				result.Evidence = append(result.Evidence, *observation.Evidence)
				evidenceIDs[observation.Evidence.ID] = true
			}
			continue
		}
		encoded, err := json.Marshal(observation.Data)
		if err != nil {
			continue
		}
		args, err := ParseActionArguments(encoded)
		if err != nil {
			continue
		}
		if args.Record != nil && !recordIDs[args.Record.ID] {
			result.Records = append(result.Records, *args.Record)
			recordIDs[args.Record.ID] = true
		}
		if args.Evidence != nil && !evidenceIDs[args.Evidence.ID] {
			result.Evidence = append(result.Evidence, *args.Evidence)
			evidenceIDs[args.Evidence.ID] = true
		}
		if args.Experiment != nil && !experimentIDs[args.Experiment.ID] {
			result.Experiments = append(result.Experiments, *args.Experiment)
			experimentIDs[args.Experiment.ID] = true
		}
	}
	if len(result.Records) == 0 && len(result.Evidence) == 0 && len(result.Experiments) == 0 {
		return nil
	}
	result.Assessment = Assess(result)
	return result
}

func Assess(current *CaseV1) Assessment {
	result := Assessment{
		MissingEvidence: []string{}, DanglingEvidence: []string{}, GateBlockers: []string{},
	}
	if current == nil {
		return result
	}
	evidence := map[string]EvidenceRef{}
	for _, item := range current.Evidence {
		evidence[item.ID] = item
	}
	for _, record := range current.Records {
		if !isMaterial(record) {
			continue
		}
		result.MaterialRecords++
		supported := len(record.EvidenceIDs) > 0
		if !supported {
			result.MissingEvidence = append(result.MissingEvidence, record.ID)
		}
		for _, evidenceID := range record.EvidenceIDs {
			item, ok := evidence[evidenceID]
			if !ok {
				result.DanglingEvidence = append(result.DanglingEvidence, record.ID+":"+evidenceID)
				supported = false
				continue
			}
			if item.Stale {
				supported = false
			}
		}
		if supported {
			result.SupportedRecords++
		}
		if record.Required {
			if !supported {
				result.GateBlockers = append(result.GateBlockers, record.ID+": required record lacks current evidence")
			}
			if record.Kind == KindProperty && record.Status != StatusPassed {
				result.GateBlockers = append(result.GateBlockers, record.ID+": required property is not passed")
			}
			if record.Kind == KindAssumption && record.Status != StatusSupported {
				result.GateBlockers = append(result.GateBlockers, record.ID+": required assumption is unresolved")
			}
		}
	}
	if result.MaterialRecords > 0 {
		result.CoveragePercent = float64(result.SupportedRecords) * 100 / float64(result.MaterialRecords)
	}
	return result
}

func isMaterial(record Record) bool {
	switch record.Kind {
	case KindFact, KindHypothesis, KindAssumption, KindProperty, KindDecision:
		return true
	default:
		return false
	}
}

func BindDiffEvidence(current *CaseV1, diffHash string) {
	if current == nil {
		return
	}
	for i := range current.Evidence {
		if current.Evidence[i].Subject == "diff" && current.Evidence[i].SubjectHash == "" {
			current.Evidence[i].SubjectHash = diffHash
		}
	}
	current.Assessment = Assess(current)
}

func FindUnresolvedUnknown(current *CaseV1, id string) (Record, bool) {
	if current == nil {
		return Record{}, false
	}
	for _, record := range current.Records {
		if record.ID == id && record.Kind == KindUnknown && record.Status == StatusUnresolved {
			return record, true
		}
	}
	return Record{}, false
}

// Observation is the narrow runtime boundary required to build a case without
// coupling this package to the agent implementation.
type Observation struct {
	Tool     string
	Status   string
	Data     any
	Evidence *EvidenceRef
}
