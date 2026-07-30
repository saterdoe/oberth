package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// RuntimeConfig describes the local API endpoint exposed to the embedded UI.
type RuntimeConfig struct {
	APIURL   string `json:"api_url"`
	APIToken string `json:"api_token"`
	Desktop  bool   `json:"desktop"`
	State    string `json:"state"`
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
}

// PickedRepository is the result of a native directory picker operation.
type PickedRepository struct {
	Canceled bool   `json:"canceled"`
	Path     string `json:"path,omitempty"`
	Name     string `json:"name,omitempty"`
}

// App owns the desktop process lifecycle and its local server connection.
type App struct {
	mu       sync.RWMutex
	ctx      context.Context
	cmd      *exec.Cmd
	config   RuntimeConfig
	log      *os.File
	stdin    io.WriteCloser
	starting bool
}

// NewApp creates an Oberth desktop application host.
func NewApp() *App {
	return &App{config: RuntimeConfig{Desktop: true, State: "preparing", Message: "Preparando el servicio local…"}}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.mu.Lock()
	a.starting = true
	a.mu.Unlock()
	if err := a.startServer(); err != nil {
		a.stopServer(10 * time.Second)
		a.mu.Lock()
		a.config.State = "error"
		a.config.Error = err.Error()
		a.config.Message = "No se pudo preparar Oberth."
		a.starting = false
		a.mu.Unlock()
		return
	}
	a.mu.Lock()
	a.config.State = "ready"
	a.config.Message = ""
	a.config.Error = ""
	a.starting = false
	a.mu.Unlock()
}

// RuntimeConfig returns the non-sensitive runtime state available to the UI.
func (a *App) RuntimeConfig() RuntimeConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.config
}

// RetryStartup retries native service preparation after a startup failure.
func (a *App) RetryStartup() RuntimeConfig {
	a.mu.Lock()
	if a.starting {
		cfg := a.config
		a.mu.Unlock()
		return cfg
	}
	a.starting = true
	a.config.State = "preparing"
	a.config.Message = "Reintentando la preparación del servicio local…"
	a.config.Error = ""
	a.mu.Unlock()

	a.stopServer(10 * time.Second)
	if err := a.startServer(); err != nil {
		a.mu.Lock()
		a.config.State = "error"
		a.config.Message = "No se pudo preparar Oberth."
		a.config.Error = err.Error()
		a.starting = false
		cfg := a.config
		a.mu.Unlock()
		return cfg
	}
	a.mu.Lock()
	a.config.State = "ready"
	a.config.Message = ""
	a.starting = false
	cfg := a.config
	a.mu.Unlock()
	return cfg
}

func (a *App) assetMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/ws/") {
			next.ServeHTTP(w, r)
			return
		}
		a.mu.RLock()
		cfg := a.config
		a.mu.RUnlock()
		target, err := url.Parse(cfg.APIURL)
		if err != nil || target.Host == "" {
			http.Error(w, "local service unavailable", http.StatusServiceUnavailable)
			return
		}
		proxy := httputil.NewSingleHostReverseProxy(target)
		originalDirector := proxy.Director
		proxy.Director = func(request *http.Request) {
			originalDirector(request)
			request.Host = target.Host
			request.Header.Set("Authorization", "Bearer "+cfg.APIToken)
		}
		proxy.ServeHTTP(w, r)
	})
}

// PickRepository opens the native repository directory picker.
func (a *App) PickRepository() (PickedRepository, error) {
	return a.PickDirectory("Seleccionar repositorio")
}

// PickDirectory opens a native directory picker with the supplied title.
func (a *App) PickDirectory(title string) (PickedRepository, error) {
	if strings.TrimSpace(title) == "" {
		title = "Seleccionar carpeta"
	}
	path, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: title,
	})
	if err != nil {
		return PickedRepository{}, err
	}
	if path == "" {
		return PickedRepository{Canceled: true}, nil
	}
	return PickedRepository{Path: path, Name: filepath.Base(path)}, nil
}

func (a *App) startServer() error {
	serverPath, err := sidecarPath()
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("reserve local API port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return fmt.Errorf("generate session token: %w", err)
	}
	token := hex.EncodeToString(rawToken)
	stateDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("resolve application data directory: %w", err)
	}
	stateDir = filepath.Join(stateDir, "oberth")
	if err := os.MkdirAll(filepath.Join(stateDir, "logs"), 0700); err != nil {
		return fmt.Errorf("create application data directory: %w", err)
	}
	logFile, err := os.OpenFile(filepath.Join(stateDir, "logs", "server.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open server log: %w", err)
	}

	cmd := exec.Command(serverPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = logFile.Close()
		return fmt.Errorf("create local service lifetime pipe: %w", err)
	}
	cmd.Dir = stateDir
	cmd.Env = append(os.Environ(),
		"OBERTH_AUTH_TOKEN="+token,
		"OBERTH_SERVER_HOST=127.0.0.1",
		"OBERTH_SERVER_PORT="+strconv.Itoa(port),
		"OBERTH_PARENT_PIPE=1",
	)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	hideChildWindow(cmd)
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = logFile.Close()
		return fmt.Errorf("start local oberth service: %w", err)
	}

	apiURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	a.mu.Lock()
	a.cmd, a.log, a.stdin = cmd, logFile, stdin
	a.config.APIURL, a.config.APIToken = apiURL, token
	a.mu.Unlock()

	client := &http.Client{Timeout: 600 * time.Millisecond}
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, apiURL+"/api/v1/health", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		if response, requestErr := client.Do(req); requestErr == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			return errors.New("local oberth service exited during startup; review logs/server.log")
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("local oberth service did not become ready in 45 seconds")
}

func (a *App) shutdown(context.Context) {
	a.stopServer(45 * time.Second)
}

func (a *App) stopServer(timeout time.Duration) {
	a.mu.RLock()
	cmd, cfg, stdin, logFile := a.cmd, a.config, a.stdin, a.log
	a.mu.RUnlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	req, _ := http.NewRequest(http.MethodPost, cfg.APIURL+"/api/v1/system/shutdown", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.APIToken)
	client := &http.Client{Timeout: 2 * time.Second}
	if response, err := client.Do(req); err == nil {
		_ = response.Body.Close()
	}
	// Closing the lifetime pipe is a fallback signal if HTTP shutdown could not
	// be delivered. The extended wait lets HTTP, DB pools and embedded
	// PostgreSQL stop in their registered order before a forced termination.
	if stdin != nil {
		_ = stdin.Close()
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
	}
	if logFile != nil {
		_ = logFile.Close()
	}
	a.mu.Lock()
	a.cmd, a.stdin, a.log = nil, nil, nil
	a.config.APIURL, a.config.APIToken = "", ""
	a.mu.Unlock()
}

func sidecarPath() (string, error) {
	name := "oberth-server"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	candidates := []string{
		filepath.Join(filepath.Dir(executable), name),
		filepath.Join(filepath.Dir(executable), "bin", name),
		filepath.Join(filepath.Dir(executable), "..", name),
	}
	for _, candidate := range candidates {
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s was not found next to the desktop application", name)
}
