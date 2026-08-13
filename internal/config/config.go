package config

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/saterdoe/oberth/internal/buildinfo"
	"github.com/spf13/viper"
)

type Config struct {
	Server            ServerConfig            `mapstructure:"server"`
	Database          DatabaseConfig          `mapstructure:"database"`
	Agent             AgentConfig             `mapstructure:"agent"`
	Context           ContextConfig           `mapstructure:"context"`
	CodeIndex         CodeIndexConfig         `mapstructure:"code_index"`
	Vault             VaultConfig             `mapstructure:"vault"`
	VectorDB          VectorDBConfig          `mapstructure:"vector_store"`
	LLM               LLMConfig               `mapstructure:"llm"`
	CostCtrl          CostCtrlConfig          `mapstructure:"cost_control"`
	Audit             AuditConfig             `mapstructure:"audit"`
	Auth              AuthConfig              `mapstructure:"auth"`
	Redis             RedisConfig             `mapstructure:"redis"`
	Python            PythonConfig            `mapstructure:"python_component"`
	StructuredOutputs StructuredOutputsConfig `mapstructure:"structured_outputs"`
}

type ServerConfig struct {
	Host      string    `mapstructure:"host"`
	Port      int       `mapstructure:"port"`
	LogLevel  string    `mapstructure:"log_level"`
	Version   string    `mapstructure:"version"`
	StartTime time.Time `mapstructure:"-"`
}

type DatabaseConfig struct {
	Driver   string `mapstructure:"driver"`
	DSN      string `mapstructure:"dsn"`
	MaxConns int    `mapstructure:"max_connections"`
}

type AgentConfig struct {
	MaxIterations int `mapstructure:"max_iterations"`
}

type ContextConfig struct {
	Mode                string `mapstructure:"mode"`
	MaxTokens           int    `mapstructure:"max_tokens"`
	ReserveOutputTokens int    `mapstructure:"reserve_output_tokens"`
	MaxSourcesPerKind   int    `mapstructure:"max_sources_per_kind"`
}

type CodeIndexConfig struct {
	Enabled       *bool    `mapstructure:"enabled"`
	MaxFileBytes  int64    `mapstructure:"max_file_bytes"`
	MaxFiles      int      `mapstructure:"max_files"`
	MaxChunks     int      `mapstructure:"max_chunks"`
	MaxChunkLines int      `mapstructure:"max_chunk_lines"`
	OverlapLines  int      `mapstructure:"overlap_lines"`
	Exclude       []string `mapstructure:"exclude"`
}

func (c CodeIndexConfig) IsEnabled() bool { return c.Enabled == nil || *c.Enabled }

type VaultConfig struct {
	Path                  string `mapstructure:"path"`
	AutoIndex             bool   `mapstructure:"auto_index"`
	IndexInterval         string `mapstructure:"index_interval"`
	ConsolidationInterval string `mapstructure:"consolidation_interval"`
	SemanticSearchLimit   int    `mapstructure:"semantic_search_limit"`
}

type VectorDBConfig struct {
	Engine   string            `mapstructure:"engine"`
	Local    LocalVectorConfig `mapstructure:"local"`
	Embedder EmbedderConfig    `mapstructure:"embedder"`
	Qdrant   QdrantConfig      `mapstructure:"qdrant"`
	ChromaDB ChromaDBConfig    `mapstructure:"chromadb"`
}

type LocalVectorConfig struct {
	Path string `mapstructure:"path"`
}

type EmbedderConfig struct {
	Provider   string `mapstructure:"provider"`
	Model      string `mapstructure:"model"`
	Dimensions int    `mapstructure:"dimensions"`
	CachePath  string `mapstructure:"cache_path"`
}

type QdrantConfig struct {
	URL        string `mapstructure:"url"`
	Collection string `mapstructure:"collection"`
}

type ChromaDBConfig struct {
	Path       string `mapstructure:"path"`
	Collection string `mapstructure:"collection"`
}

type EmbeddingsConfig struct {
	Model        string `mapstructure:"model"`
	Chunking     string `mapstructure:"chunking"`
	ChunkSize    int    `mapstructure:"chunk_size"`
	ChunkOverlap int    `mapstructure:"chunk_overlap"`
}

type LLMConfig struct {
	DefaultProvider string      `mapstructure:"default_provider"`
	DefaultModel    string      `mapstructure:"default_model"`
	AttemptTimeout  string      `mapstructure:"attempt_timeout"`
	Cache           CacheConfig `mapstructure:"cache"`
}

type CacheConfig struct {
	Prompts      bool   `mapstructure:"prompts"`
	Embeddings   bool   `mapstructure:"embeddings"`
	LLMResponses bool   `mapstructure:"llm_responses"`
	MemoryIndex  bool   `mapstructure:"memory_index"`
	TTL          string `mapstructure:"ttl"`
	MaxEntries   int    `mapstructure:"max_entries"`
}

type CostCtrlConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	AlertWebhook string `mapstructure:"alert_webhook"`
	Currency     string `mapstructure:"currency"`
}

type AuditConfig struct {
	RetentionDays int    `mapstructure:"retention_days"`
	ExportPath    string `mapstructure:"export_path"`
}

type AuthConfig struct {
	Mode              string `mapstructure:"mode"`
	Token             string `mapstructure:"token"`
	ProviderSecretKey string `mapstructure:"-"`
}

type RedisConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	URL     string `mapstructure:"url"`
}

type PythonConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	URL     string `mapstructure:"url"`
}

type StructuredOutputsConfig struct {
	Engine  string       `mapstructure:"engine"`
	Enabled bool         `mapstructure:"enabled"`
	BAML    BAMLConfig   `mapstructure:"baml"`
	Traces  TracesConfig `mapstructure:"traces"`
}

type BAMLConfig struct {
	SrcPath        string               `mapstructure:"src_path"`
	GeneratedPath  string               `mapstructure:"generated_path"`
	BoundaryStudio BoundaryStudioConfig `mapstructure:"boundary_studio"`
}

type BoundaryStudioConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Mode    string `mapstructure:"mode"`
}

type TracesConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	Path       string `mapstructure:"path"`
	RetainDays int    `mapstructure:"retain_days"`
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.AutomaticEnv()
	v.SetEnvPrefix("PI")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func Default() *Config {
	vaultPath := "./.agent-vault"
	localVectorPath := "./data/vector/index.json"
	embeddingCachePath := "./data/vector/embeddings.json"
	if userConfigDir, err := os.UserConfigDir(); err == nil {
		appDataPath := filepath.Join(userConfigDir, "oberth")
		vaultPath = filepath.Join(appDataPath, ".agent-vault")
		localVectorPath = filepath.Join(appDataPath, "vector", "index.json")
		embeddingCachePath = filepath.Join(appDataPath, "vector", "embeddings.json")
	}
	attemptTimeout := strings.TrimSpace(os.Getenv("OBERTH_LLM_ATTEMPT_TIMEOUT"))
	if attemptTimeout == "" {
		attemptTimeout = "10m"
	}
	return &Config{
		Server: ServerConfig{
			Host:     "127.0.0.1",
			Port:     9090,
			LogLevel: "info",
			Version:  buildinfo.Version,
		},
		Database: DatabaseConfig{
			Driver:   "embedded",
			DSN:      "",
			MaxConns: 20,
		},
		Agent: AgentConfig{
			MaxIterations: 12,
		},
		Context: ContextConfig{
			Mode:                "dev",
			MaxTokens:           8000,
			ReserveOutputTokens: 2000,
			MaxSourcesPerKind:   6,
		},
		CodeIndex: CodeIndexConfig{Enabled: boolPtr(true), MaxFileBytes: 512 * 1024, MaxFiles: 5000, MaxChunks: 20000, MaxChunkLines: 240, OverlapLines: 20},
		Vault: VaultConfig{
			Path:                  vaultPath,
			AutoIndex:             true,
			IndexInterval:         "5m",
			ConsolidationInterval: "24h",
			SemanticSearchLimit:   3,
		},
		VectorDB: VectorDBConfig{
			Engine: "builtin",
			Local: LocalVectorConfig{
				Path: localVectorPath,
			},
			Embedder: EmbedderConfig{
				Provider:   "builtin",
				Model:      "pi-feature-hash-v1",
				Dimensions: 384,
				CachePath:  embeddingCachePath,
			},
			Qdrant: QdrantConfig{
				URL:        "http://localhost:6333",
				Collection: "oberth-notes",
			},
			ChromaDB: ChromaDBConfig{
				Path:       "./data/chromadb",
				Collection: "oberth-notes",
			},
		},
		LLM: LLMConfig{
			DefaultProvider: "openai",
			DefaultModel:    "gpt-4o-mini",
			AttemptTimeout:  attemptTimeout,
			Cache: CacheConfig{
				Prompts:      true,
				Embeddings:   true,
				LLMResponses: true,
				MemoryIndex:  true,
				TTL:          "10m",
				MaxEntries:   1000,
			},
		},
		CostCtrl: CostCtrlConfig{
			Enabled:  true,
			Currency: "USD",
		},
		Audit: AuditConfig{
			RetentionDays: 90,
			ExportPath:    "./data/audit",
		},
		Auth: AuthConfig{
			Mode: "none",
		},
		Redis: RedisConfig{
			Enabled: false,
			URL:     "redis://localhost:6379",
		},
		Python: PythonConfig{
			Enabled: false,
			URL:     "http://localhost:8000",
		},
		StructuredOutputs: StructuredOutputsConfig{
			Engine:  "native_json",
			Enabled: false,
			BAML: BAMLConfig{
				SrcPath:       "./baml_src",
				GeneratedPath: "./internal/generated/baml",
				BoundaryStudio: BoundaryStudioConfig{
					Enabled: false,
					Mode:    "off",
				},
			},
			Traces: TracesConfig{
				Enabled:    true,
				Path:       "./data/traces",
				RetainDays: 30,
			},
		},
	}
}

func boolPtr(value bool) *bool { return &value }
