package structuredoutput

// ── ClassifyTask ──────────────────────────────────────────────

type ClassifyTaskInput struct {
	TaskDescription string            `json:"task_description"`
	RepoMetadata    map[string]string `json:"repo_metadata,omitempty"`
	CurrentBranch   string            `json:"current_branch,omitempty"`
	OptionalFiles   []string          `json:"optional_files,omitempty"`
}

type ClassifyTaskOutput struct {
	TaskType               string  `json:"task_type"`
	Confidence             float64 `json:"confidence"`
	RiskLevel              string  `json:"risk_level"`
	RequiresGitChanges     bool    `json:"requires_git_changes"`
	RequiresHumanApproval  bool    `json:"requires_human_approval"`
	SuggestedExecutionMode string  `json:"suggested_execution_mode"`
}

// ── SelectContext ─────────────────────────────────────────────

type SelectContextInput struct {
	Task            string   `json:"task"`
	TaskDescription string   `json:"task_description"`
	MemoryIndex     []string `json:"memory_index,omitempty"`
	CandidateNotes  []string `json:"candidate_notes,omitempty"`
	RepoMap         string   `json:"repo_map,omitempty"`
}

type SelectContextOutput struct {
	SelectedSources []string `json:"selected_sources"`
	RejectedSources []string `json:"rejected_sources"`
	Reason          string   `json:"reason"`
	Confidence      float64  `json:"confidence"`
	EstimatedTokens int      `json:"estimated_tokens"`
}

// ── ReviewPatch ───────────────────────────────────────────────

type ReviewPatchInput struct {
	Task            string   `json:"task"`
	TaskDescription string   `json:"task_description"`
	Patch           string   `json:"patch"`
	TestOutput      string   `json:"test_output,omitempty"`
	ContextSources  []string `json:"context_sources,omitempty"`
}

type FileComment struct {
	FilePath string `json:"file_path"`
	Line     int    `json:"line"`
	Comment  string `json:"comment"`
	Severity string `json:"severity"`
}

type ReviewPatchOutput struct {
	Approved              bool          `json:"approved"`
	Severity              string        `json:"severity"`
	BlockingIssues        []string      `json:"blocking_issues"`
	CommentsByFile        []FileComment `json:"comments_by_file"`
	RequiredChanges       []string      `json:"required_changes"`
	ShouldRetryGeneration bool          `json:"should_retry_generation"`
}

// ── DetectADRCandidate ────────────────────────────────────────

type DetectADRCandidateInput struct {
	SessionSummary   string   `json:"session_summary"`
	GeneratedChanges string   `json:"generated_changes,omitempty"`
	UserDecisions    []string `json:"user_decisions,omitempty"`
}

type DetectADRCandidateOutput struct {
	ShouldCreateADR bool    `json:"should_create_adr"`
	Title           string  `json:"title,omitempty"`
	Decision        string  `json:"decision,omitempty"`
	Context         string  `json:"context,omitempty"`
	Alternatives    string  `json:"alternatives,omitempty"`
	Consequences    string  `json:"consequences,omitempty"`
	Confidence      float64 `json:"confidence"`
}

// ── ScoreRisk ─────────────────────────────────────────────────

type ScoreRiskInput struct {
	Task               string            `json:"task"`
	TaskDescription    string            `json:"task_description"`
	FilesToModify      []string          `json:"files_to_modify,omitempty"`
	RepoClassification string            `json:"repo_classification,omitempty"`
	SelectedModel      string            `json:"selected_model,omitempty"`
	PolicyContext      map[string]string `json:"policy_context,omitempty"`
}

type ScoreRiskOutput struct {
	RiskLevel        string   `json:"risk_level"`
	Reasons          []string `json:"reasons"`
	RequiresApproval bool     `json:"requires_approval"`
	CloudAllowed     bool     `json:"cloud_allowed"`
	RequiredChecks   []string `json:"required_checks"`
}
