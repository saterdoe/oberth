package main

import (
	"testing"

	"github.com/saterdoe/oberth/internal/buildinfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionCmd_Output(t *testing.T) {
	rootCmd.SetArgs([]string{"version"})
	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestVersionCmd_DefaultValues(t *testing.T) {
	assert.Equal(t, buildinfo.Version, Version)
	assert.Equal(t, "unknown", Commit)
	assert.Equal(t, "unknown", BuildDate)
}
