package localprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/saterdoe/oberth/internal/agentadapter"
)

type Candidate struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	ProviderType string   `json:"provider_type,omitempty"`
	BaseURL      string   `json:"base_url,omitempty"`
	Installed    bool     `json:"installed"`
	Running      bool     `json:"running"`
	Usable       bool     `json:"usable"`
	Models       []string `json:"models"`
	Message      string   `json:"message"`
	Kind         string   `json:"kind"`
	Version      string   `json:"version,omitempty"`
	Auth         string   `json:"auth,omitempty"`
	Evidence     []string `json:"evidence,omitempty"`
}

type discoverer struct {
	lookPath func(string) (string, error)
	client   *http.Client
	goos     string
}

func Discover(ctx context.Context) []Candidate {
	d := discoverer{
		lookPath: exec.LookPath,
		client:   &http.Client{Timeout: 1500 * time.Millisecond},
		goos:     runtime.GOOS,
	}
	return d.discover(ctx)
}

func (d discoverer) discover(ctx context.Context) []Candidate {
	ollama := Candidate{ID: "ollama", Name: "Ollama", Kind: "inference-provider", ProviderType: "ollama", BaseURL: "http://127.0.0.1:11434", Models: []string{}}
	_, ollamaErr := d.lookPath("ollama")
	ollama.Installed = ollamaErr == nil
	ollama.Models, ollama.Running = d.ollamaModels(ctx, ollama.BaseURL)
	ollama.Usable = ollama.Running && len(ollama.Models) > 0
	ollama.Message = candidateMessage(ollama, "Instalá al menos un modelo con Ollama para poder usarlo.")

	lmStudio := Candidate{ID: "lm-studio", Name: "LM Studio", Kind: "inference-provider", ProviderType: "custom", BaseURL: "http://127.0.0.1:1234/v1", Models: []string{}}
	_, lmsErr := d.lookPath("lms")
	lmStudio.Installed = lmsErr == nil || d.lmStudioAppInstalled()
	lmStudio.Models, lmStudio.Running = d.openAIModels(ctx, lmStudio.BaseURL)
	lmStudio.Usable = lmStudio.Running && len(lmStudio.Models) > 0
	lmStudio.Message = candidateMessage(lmStudio, "Iniciá el servidor local y cargá un modelo desde LM Studio.")

	agents := agentadapter.DefaultAgents()
	candidates := []Candidate{ollama, lmStudio}
	for _, adapter := range agents {
		capability, _ := adapter.Capabilities(ctx)
		candidates = append(candidates, Candidate{
			ID: adapter.Name(), Name: agentDisplayName(adapter.Name()), Kind: "agent-harness",
			Installed: capability.Installed, Running: capability.Usable, Usable: capability.Usable,
			Message: capability.Message, Version: capability.Version, Auth: capability.Auth,
			Evidence: capability.Evidence, Models: []string{},
		})
	}
	for i, c := range candidates {
		if c.ID == "claude-code" && !c.Installed && d.claudeDesktopInstalled() {
			candidates[i].Name = "Claude Desktop"
			candidates[i].Installed = true
			candidates[i].Message = "Claude Desktop está instalado, pero no reemplaza Claude Code ni expone un harness compatible."
		}
	}
	return candidates
}

func agentDisplayName(id string) string {
	switch id {
	case "codex":
		return "Codex CLI"
	case "claude-code":
		return "Claude Code"
	case "opencode":
		return "OpenCode"
	case "antigravity":
		return "Antigravity"
	default:
		return id
	}
}

func candidateMessage(candidate Candidate, installedHelp string) string {
	if candidate.Usable {
		return "Listo para usar."
	}
	if candidate.Running {
		return installedHelp
	}
	if candidate.Installed {
		return installedHelp
	}
	return "No detectado en este equipo."
}

func (d discoverer) ollamaModels(ctx context.Context, baseURL string) ([]string, bool) {
	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if !d.getJSON(ctx, baseURL+"/api/tags", &payload) {
		return []string{}, false
	}
	models := make([]string, 0, len(payload.Models))
	for _, model := range payload.Models {
		if model.Name != "" {
			models = append(models, model.Name)
		}
	}
	return models, true
}

func (d discoverer) openAIModels(ctx context.Context, baseURL string) ([]string, bool) {
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if !d.getJSON(ctx, baseURL+"/models", &payload) {
		return []string{}, false
	}
	models := make([]string, 0, len(payload.Data))
	for _, model := range payload.Data {
		if model.ID != "" {
			models = append(models, model.ID)
		}
	}
	return models, true
}

func (d discoverer) getJSON(ctx context.Context, url string, target any) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	response, err := d.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode >= 200 && response.StatusCode < 300 && json.NewDecoder(response.Body).Decode(target) == nil
}

func (d discoverer) lmStudioAppInstalled() bool {
	switch d.goos {
	case "windows":
		home, _ := os.UserHomeDir()
		_, err := os.Stat(home + `\.lmstudio\bin\lms.exe`)
		return err == nil
	case "darwin":
		_, err := os.Stat("/Applications/LM Studio.app")
		return err == nil
	default:
		_, err := d.lookPath("lm-studio")
		return err == nil
	}
}

func (d discoverer) claudeDesktopInstalled() bool {
	switch d.goos {
	case "windows":
		output, err := exec.Command("tasklist", "/FI", "IMAGENAME eq Claude.exe", "/FO", "CSV", "/NH").Output()
		return err == nil && strings.Contains(strings.ToLower(string(output)), "claude.exe")
	case "darwin":
		_, err := os.Stat("/Applications/Claude.app")
		return err == nil
	default:
		_, err := d.lookPath("claude-desktop")
		return err == nil
	}
}
