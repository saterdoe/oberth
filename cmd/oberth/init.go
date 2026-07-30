package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

type initFlags struct {
	force bool
}

var initFlag = initFlags{}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize oberth in the current project",
	RunE:  initRun,
}

func initRun(cmd *cobra.Command, args []string) error {
	vaultDir := ".agent-vault"
	configFile := ".oberth.yaml"

	if !initFlag.force {
		if _, err := os.Stat(vaultDir); err == nil {
			return fmt.Errorf("%s already exists (use --force to overwrite)", vaultDir)
		}
		if _, err := os.Stat(configFile); err == nil {
			return fmt.Errorf("%s already exists (use --force to overwrite)", configFile)
		}
	}

	subdirs := []string{"architecture", "decisions", "patterns", "bugs", "sessions", "tasks"}
	for _, dir := range subdirs {
		if err := os.MkdirAll(filepath.Join(vaultDir, dir), 0755); err != nil {
			return fmt.Errorf("creating %s/%s: %w", vaultDir, dir, err)
		}
	}

	memoryIndex := filepath.Join(vaultDir, "memory-index.md")
	if err := os.WriteFile(memoryIndex, []byte("# Memory Index\n\n"), 0644); err != nil {
		return fmt.Errorf("creating memory-index.md: %w", err)
	}

	if err := os.WriteFile(configFile, []byte(defaultConfigYAML), 0600); err != nil {
		return fmt.Errorf("creating %s: %w", configFile, err)
	}

	fmt.Printf("Created %s/\n", vaultDir)
	for _, dir := range subdirs {
		fmt.Printf("  %s/\n", filepath.Join(vaultDir, dir))
	}
	fmt.Printf("  %s\n", memoryIndex)
	fmt.Printf("Created %s\n", configFile)
	fmt.Println()
	fmt.Println("oberth initialized. Run `oberth doctor` to verify.")

	return nil
}

const defaultConfigYAML = `server:
  host: 127.0.0.1
  port: 9090
  log_level: info

database:
  driver: embedded
  dsn: ""
  max_connections: 20

agent:
  max_iterations: 12

context:
  mode: dev
  max_tokens: 8000
  reserve_output_tokens: 2000
  max_sources_per_kind: 6

vault:
  path: ./.agent-vault
  auto_index: true
  index_interval: 5m

vector_store:
  engine: builtin
  local:
    path: ./data/vector/index.json
  embedder:
    provider: builtin
    model: pi-feature-hash-v1
    dimensions: 384
    cache_path: ./data/vector/embeddings.json

llm:
  default_provider: openai
  default_model: gpt-4o-mini
  cache:
    prompts: true
    embeddings: true
    llm_responses: true
    memory_index: true
    ttl: 10m
    max_entries: 1000

cost_control:
  enabled: true
  currency: USD

audit:
  retention_days: 90
  export_path: ./data/audit

auth:
  mode: token

redis:
  enabled: false
  url: redis://localhost:6379

python_component:
  enabled: false
  url: http://localhost:8000

structured_outputs:
  enabled: false
  engine: native_json
`

func init() {
	initCmd.Flags().BoolVarP(&initFlag.force, "force", "f", false, "overwrite existing files")
}
