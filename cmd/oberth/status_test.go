package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStatusCmdWithoutDaemon(t *testing.T) {
	previousPort := apiPort
	apiPort = 1
	defer func() { apiPort = previousPort }()
	rootCmd.SetArgs([]string{"status"})
	require.Error(t, rootCmd.Execute())
}

func TestLegacyLifecycleCommandsAreRemoved(t *testing.T) {
	for _, command := range []string{"up", "down", "vault", "session", "cost"} {
		rootCmd.SetArgs([]string{command})
		require.Error(t, rootCmd.Execute())
	}
}
