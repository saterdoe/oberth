package vault

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoteUsesFrontendJSONContract(t *testing.T) {
	encoded, err := json.Marshal(Note{
		Path:     "projects/demo/session",
		Content:  "memory",
		Metadata: map[string]any{"type": "session"},
	})
	require.NoError(t, err)
	require.JSONEq(t, `{
		"path":"projects/demo/session",
		"content":"memory",
		"metadata":{"type":"session"}
	}`, string(encoded))
}
