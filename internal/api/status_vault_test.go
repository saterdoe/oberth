package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/saterdoe/oberth/internal/config"
	"github.com/saterdoe/oberth/internal/vault"
	"github.com/stretchr/testify/require"
)

func TestStatusCountsNestedVaultNotesAndReportsDisabledSemanticSearch(t *testing.T) {
	v := vault.New(t.TempDir())
	require.NoError(t, v.Ensure())
	_, err := v.CreateNote("memory-index", "index", nil)
	require.NoError(t, err)
	_, err = v.CreateNote("projects/demo/sessions/run", "memory", nil)
	require.NoError(t, err)

	server := &Server{
		cfg:       &config.Config{VectorDB: config.VectorDBConfig{Engine: "disabled"}},
		vaultConn: v,
	}
	response := httptest.NewRecorder()
	server.handleGetStatus(response, httptest.NewRequest("GET", "/api/v1/status", nil))

	require.Equal(t, 200, response.Code)
	var envelope struct {
		Data struct {
			VectorStore struct {
				State string `json:"state"`
			} `json:"vector_store"`
			Vault struct {
				NoteCount int `json:"note_count"`
			} `json:"vault"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	require.Equal(t, "disabled", envelope.Data.VectorStore.State)
	require.Equal(t, 2, envelope.Data.Vault.NoteCount)
}
