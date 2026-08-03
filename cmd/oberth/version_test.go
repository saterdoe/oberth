package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionCmd_Output(t *testing.T) {
	rootCmd.SetArgs([]string{"version"})
	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestVersionCmd_DefaultValues(t *testing.T) {
	assert.Equal(t, "0.1.0-alpha.2", Version)
	assert.Equal(t, "unknown", Commit)
	assert.Equal(t, "unknown", BuildDate)
}
