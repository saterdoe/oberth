package context_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/saterdoe/oberth/internal/context"
	"github.com/saterdoe/oberth/internal/vault"
	"github.com/stretchr/testify/require"
)

func TestRepositoryCodeIndexFeedsBudgetedContextWithManifest(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repo, "compiler.go"), []byte("package compiler\n// CompileContext selects memory sources sent to the model.\nfunc CompileContext() {}\n"), 0600))
	// Keep the persistent derived fixture out of the developer's normal cache.
	t.Setenv("LOCALAPPDATA", t.TempDir())
	p := pkgctx.NewPipeline(vault.New(t.TempDir()), nil)
	result, err := p.CompileRepository(context.Background(), repo, "where are memory sources sent to the model", "implementation", pkgctx.CompileOptions{MaxTokens: 160, ReserveOutputTokens: 40, MaxSourcesPerKind: 4})
	require.NoError(t, err)
	require.LessOrEqual(t, result.Tokens, 120)
	require.Contains(t, result.Context, "CompileContext")
	found := false
	for _, m := range result.Manifest {
		if strings.Contains(m.ID, "compiler.go") && strings.Contains(m.Reason, "lexical") {
			found = true
		}
	}
	require.True(t, found, "code index selection must be visible in the context manifest")
}
