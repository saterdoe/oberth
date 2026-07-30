package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProviderVerificationRequestAllowsReasoningBeforeVisibleOutput(t *testing.T) {
	request := providerVerificationRequest("gemma4:12b")

	require.Equal(t, "gemma4:12b", request.Model)
	require.Equal(t, "Reply with exactly: OK", request.Messages[0].Content)
	require.GreaterOrEqual(t, request.MaxTokens, 128)
}
