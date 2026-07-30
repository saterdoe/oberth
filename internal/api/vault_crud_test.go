package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/saterdoe/oberth/internal/vault"
)

func TestVaultCRUDHandlers(t *testing.T) {
	s := &Server{vaultConn: vault.New(t.TempDir())}
	create := httptest.NewRecorder()
	s.handleCreateVaultNote(create, httptest.NewRequest("POST", "/api/v1/vault/notes", strings.NewReader(`{"path":"decisions/demo","content":"first","metadata":{"type":"decision"}}`)))
	if create.Code != 201 {
		t.Fatalf("create=%d %s", create.Code, create.Body.String())
	}
	updateReq := httptest.NewRequest("PUT", "/api/v1/vault/notes/decisions/demo", strings.NewReader(`{"content":"second","metadata":{"type":"decision"}}`))
	updateReq.SetPathValue("path", "decisions/demo")
	update := httptest.NewRecorder()
	s.handleUpdateVaultNote(update, updateReq)
	if update.Code != 200 {
		t.Fatalf("update=%d %s", update.Code, update.Body.String())
	}
	removeReq := httptest.NewRequest("DELETE", "/api/v1/vault/notes/decisions/demo", nil)
	removeReq.SetPathValue("path", "decisions/demo")
	remove := httptest.NewRecorder()
	s.handleDeleteVaultNote(remove, removeReq)
	if remove.Code != 200 {
		t.Fatalf("delete=%d %s", remove.Code, remove.Body.String())
	}
}
