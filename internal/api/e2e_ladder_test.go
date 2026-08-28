//go:build e2e

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/saterdoe/oberth/internal/db"
	"github.com/saterdoe/oberth/pkg/llm"
)

func oracleRequest(t *testing.T, method, url string, body any, want int) map[string]any {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(method, url, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != want {
		t.Fatalf("%s %s: got %d want %d: %+v", method, url, response.StatusCode, want, result)
	}
	return result
}

func exerciseDecisionLadder(t *testing.T, url, repository, projectID, recoveredTaskID, oldRunID string, fake *durableFakeProvider) {
	t.Helper()
	initial, err := os.ReadFile(filepath.Join(repository, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	unchanged := func() {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(repository, "main.go"))
		if err != nil || !bytes.Equal(data, initial) {
			t.Fatalf("unapproved operation changed primary checkout: %s (%v)", data, err)
		}
	}
	queueChange := func() {
		fake.append(toolResponse("read", `{"path":"main.go"}`), toolResponse("patch", `{"path":"main.go","old_text":"return \"new\"","new_text":"return \"revised\""}`), toolResponse("command", `{"program":"git","args":["diff","--check"]}`), toolResponse("finish", `{"summary":"Changed Message to revised and verified the diff."}`))
	}
	start := func(id string) map[string]any {
		t.Helper()
		queueChange()
		accepted := postData(t, url+"/api/v1/tasks/"+id+"/run", map[string]any{})
		return waitForRunState(t, url, accepted["run_id"].(string), "review")
	}
	decide := func(run map[string]any, outcome string) map[string]any {
		t.Helper()
		return postData(t, url+"/api/v1/runs/"+run["id"].(string)+"/outcome", map[string]any{"outcome": outcome})
	}

	t.Log("GATE resume: recovered task starts a new isolated attempt and replays durable events")
	resumed := start(recoveredTaskID)
	if resumed["id"] == oldRunID {
		t.Fatal("resume reused interrupted run")
	}
	page := getData(t, url+"/api/v1/runs/"+resumed["id"].(string)+"/events?after=0").(map[string]any)
	events := page["events"].([]any)
	if len(events) < 3 {
		t.Fatalf("missing durable events: %+v", page)
	}
	cursor := events[0].(map[string]any)["sequence"].(float64)
	replay := getData(t, fmt.Sprintf("%s/api/v1/runs/%s/events?after=%.0f", url, resumed["id"], cursor)).(map[string]any)["events"].([]any)
	for _, event := range replay {
		if event.(map[string]any)["sequence"].(float64) <= cursor {
			t.Fatal("reconnect duplicated acknowledged event")
		}
	}
	decide(resumed, "rejected")
	unchanged()

	t.Log("GATE planning: read-only plan completes without approval or file writes")
	fake.append(&llm.ChatResponse{Model: "fake-model", Content: "Plan: change Message in main.go to return revised; verify with git diff --check. Await approval."})
	task := postData(t, url+"/api/v1/tasks", map[string]any{"repository_id": projectID, "title": "Plan a tiny change", "description": "Propose a plan only. Do not modify files.", "task_type": "analysis", "constraints": map[string]any{"interaction_mode": "plan"}})
	id := task["id"].(string)
	planStart := postData(t, url+"/api/v1/tasks/"+id+"/run", map[string]any{})
	plan := waitForRunState(t, url, planStart["run_id"].(string), "completed")
	encoded, _ := json.Marshal(plan["result_bundle"])
	if !strings.Contains(string(encoded), "Await approval") {
		t.Fatal("plan answer missing from result bundle")
	}
	unchanged()
	oracleRequest(t, http.MethodPut, url+"/api/v1/tasks/"+id, map[string]any{"description": "Approved: implement the discussed one-file change.", "task_type": "implementation", "constraints": map[string]any{"interaction_mode": "implementation"}}, 200)

	t.Log("GATE correction: decision preserves main and creates a distinct next attempt")
	first := start(id)
	decide(first, "corrected")
	unchanged()
	second := start(id)
	if first["id"] == second["id"] || first["worktree_path"] == second["worktree_path"] {
		t.Fatal("correction did not create a fresh isolated attempt")
	}

	t.Log("GATE conflict: dirty main rejects promotion and retains the review")
	writeFixtureFile(t, repository, "main.go", string(initial)+"\n// uncommitted local edit\n")
	oracleRequest(t, http.MethodPost, url+"/api/v1/runs/"+second["id"].(string)+"/outcome", map[string]any{"outcome": "accepted"}, 409)
	writeFixtureFile(t, repository, "main.go", string(initial))
	decide(second, "rejected")
	unchanged()

	t.Log("GATE planned-promotion: approved plan produces exactly the reviewed file content")
	finalTask := postData(t, url+"/api/v1/tasks", map[string]any{"repository_id": projectID, "title": "Approved tiny change", "description": "Implement the approved change and verify it", "task_type": "implementation"})
	finalRun := start(finalTask["id"].(string))
	promotion := decide(finalRun, "accepted")["promotion"].(map[string]any)
	final, err := os.ReadFile(filepath.Join(repository, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(final) != strings.Replace(string(initial), `return "new"`, `return "revised"`, 1) {
		t.Fatalf("unexpected promoted content: %s", final)
	}
	commit, ok := promotion["commit"].(string)
	if !ok || commit == "" {
		t.Fatal("promotion did not report its commit")
	}
	command := exec.Command("git", "show", commit+":main.go")
	command.Dir = repository
	committed, err := command.Output()
	if err != nil || !bytes.Equal(committed, final) {
		t.Fatalf("promoted commit differs from checkout: %v", err)
	}
}

// Preparation is an explicit online action, separate from the offline oracle.
func TestPrepareHermeticDatabase(t *testing.T) {
	root := os.Getenv("OBERTH_E2E_PREPARE_DIR")
	if root == "" {
		t.Skip("explicit dependency preparation only")
	}
	embedded, err := db.StartEmbedded(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := embedded.Stop(); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		// The embedded library probes pg_ctl without .exe before deciding to
		// download. A copy in our prepared fixture cache avoids that fetch path.
		binary, err := os.ReadFile(filepath.Join(root, "v16", "binaries", "bin", "pg_ctl.exe"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "v16", "binaries", "bin", "pg_ctl"), binary, 0755); err != nil {
			t.Fatal(err)
		}
	}
}

func startOracleDatabase(t *testing.T, root string) *db.Embedded {
	t.Helper()
	if binaries := os.Getenv("OBERTH_E2E_POSTGRES_BIN"); binaries != "" {
		// Deny non-loopback HTTP, including accidental inference or download calls.
		previous := http.DefaultTransport
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ip := net.ParseIP(host)
			if ip == nil || !ip.IsLoopback() {
				return nil, fmt.Errorf("hermetic gate denied non-loopback network")
			}
			return (&net.Dialer{}).DialContext(ctx, network, address)
		}
		http.DefaultTransport = transport
		t.Cleanup(func() { transport.CloseIdleConnections(); http.DefaultTransport = previous })
		embedded, err := db.StartEmbeddedOffline(root, binaries)
		if err != nil {
			t.Fatal(err)
		}
		return embedded
	}
	embedded, err := db.StartEmbedded(root)
	if err != nil {
		t.Fatal(err)
	}
	return embedded
}

func retainOracleArtifacts(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	root := os.Getenv("OBERTH_E2E_ARTIFACT_DIR")
	if root == "" {
		return
	}
	t.Cleanup(func() {
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Error(err)
			return
		}
		for name, query := range map[string]string{
			"runs.json":   `SELECT COALESCE(json_agg(json_build_object('id',id,'state',state,'result_bundle',result_bundle)),'[]') FROM task_runs`,
			"events.json": `SELECT COALESCE(json_agg(json_build_object('run_id',run_id,'sequence',sequence,'type',event_type,'payload',payload) ORDER BY run_id,sequence),'[]') FROM run_events`,
		} {
			var data json.RawMessage
			if err := pool.QueryRow(context.Background(), query).Scan(&data); err != nil {
				t.Error(err)
				continue
			}
			if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
				t.Error(err)
			}
		}
	})
}
