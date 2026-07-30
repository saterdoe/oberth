package permission

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var (
	ErrOutsideWorkspace = errors.New("path is outside workspace")
	ErrSymlinkEscape    = errors.New("symlink resolves outside workspace")
)

type WorkspaceGuard struct {
	root         string
	resolvedRoot string
}

func NewWorkspaceGuard(root string) (*WorkspaceGuard, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}
	return &WorkspaceGuard{root: filepath.Clean(abs), resolvedRoot: filepath.Clean(resolved)}, nil
}

func (g *WorkspaceGuard) Authorize(target string) error {
	abs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if !within(g.root, abs) {
		return ErrOutsideWorkspace
	}
	resolved, err := resolveExistingPrefix(abs)
	if err != nil {
		return err
	}
	if !within(g.resolvedRoot, resolved) {
		return ErrSymlinkEscape
	}
	return nil
}

func within(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func resolveExistingPrefix(path string) (string, error) {
	current := filepath.Clean(path)
	var suffix []string
	for {
		if _, err := os.Lstat(current); err == nil {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", err
			}
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", os.ErrNotExist
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

type AuditEvent struct {
	Action  string `json:"action"`
	Summary string `json:"summary"`
	Granted bool   `json:"granted"`
	Persist bool   `json:"persist"`
}

type SecurityPolicy struct {
	audit             func(AuditEvent)
	persistentCloudOK bool
	storePath         string
}

func NewSecurityPolicy(audit func(AuditEvent)) *SecurityPolicy {
	return &SecurityPolicy{audit: audit}
}

func NewSecurityPolicyWithStore(path string, audit func(AuditEvent)) (*SecurityPolicy, error) {
	policy := &SecurityPolicy{audit: audit, storePath: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return policy, nil
		}
		return nil, fmt.Errorf("read cloud consent: %w", err)
	}
	var state struct {
		CloudAllowed bool `json:"cloud_allowed"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("decode cloud consent: %w", err)
	}
	policy.persistentCloudOK = state.CloudAllowed
	return policy, nil
}

func (p *SecurityPolicy) Authorize(operation, _ string) Decision {
	switch {
	case strings.HasPrefix(operation, "network.") || operation == "web":
		return Deny
	case strings.HasPrefix(operation, "cloud."):
		if p.persistentCloudOK {
			return Allow
		}
		return Ask
	default:
		return defaultDecision(operation)
	}
}

func (p *SecurityPolicy) CloudConsent(summary string, persist, granted bool) bool {
	if p.audit != nil {
		p.audit(AuditEvent{Action: "cloud_consent", Summary: summary, Granted: granted, Persist: persist})
	}
	if granted && persist {
		p.persistentCloudOK = true
		_ = p.persistConsent()
	}
	return granted
}

func (p *SecurityPolicy) persistConsent() error {
	if p.storePath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p.storePath), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(struct {
		CloudAllowed bool `json:"cloud_allowed"`
	}{p.persistentCloudOK})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p.storePath), ".consent-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, p.storePath)
}

type Change struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type SecurityFinding struct {
	Category    string `json:"category"`
	Path        string `json:"path"`
	Evidence    string `json:"evidence"`
	Remediation string `json:"remediation"`
}

type SecurityReview struct {
	Scorecard string            `json:"scorecard"`
	Findings  []SecurityFinding `json:"findings"`
}

func ReviewChanges(changes []Change) SecurityReview {
	review := SecurityReview{Scorecard: "READY", Findings: []SecurityFinding{}}
	for _, change := range changes {
		lowerPath := strings.ToLower(filepath.ToSlash(change.Path))
		lower := strings.ToLower(change.Content)
		if strings.HasSuffix(lowerPath, ".env") || strings.Contains(lower, "api_key=") || strings.Contains(lower, "password=") {
			review.Findings = append(review.Findings, SecurityFinding{"secrets", change.Path, "change contains a secret-like assignment", "remove the credential and load it from the secret store"})
		}
		if strings.Contains(lower, "curl ") && (strings.Contains(lower, "| sh") || strings.Contains(lower, "| bash")) {
			review.Findings = append(review.Findings, SecurityFinding{"command_injection", change.Path, "remote content is piped directly to a shell", "download, verify and execute an allow-listed artifact"})
		}
		if strings.Contains(lower, "chmod 777") || strings.Contains(lower, "0o777") {
			review.Findings = append(review.Findings, SecurityFinding{"permissions", change.Path, "world-writable permission detected", "use the minimum required file permission"})
		}
		if strings.Contains(lowerPath, "go.mod") || strings.Contains(lowerPath, "package-lock.json") || strings.Contains(lowerPath, "requirements") {
			review.Findings = append(review.Findings, SecurityFinding{"dependencies", change.Path, "dependency manifest changed", "review provenance and vulnerability scan results"})
		}
	}
	if len(review.Findings) > 0 {
		review.Scorecard = "NOT_READY"
	}
	return review
}

func (r SecurityReview) Categories() []string {
	seen := map[string]bool{}
	for _, finding := range r.Findings {
		seen[finding.Category] = true
	}
	result := make([]string, 0, len(seen))
	for category := range seen {
		result = append(result, category)
	}
	sort.Strings(result)
	return result
}
