package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultModels_ContainsCurrentGPT5Models(t *testing.T) {
	ids := DefaultModelIDs()

	require.Contains(t, ids, "gpt-5.6-sol")
	require.Contains(t, ids, "gpt-5.6-terra")
	require.Contains(t, ids, "gpt-5.6-luna")
	require.Contains(t, ids, "gpt-5.5")
	require.Contains(t, ids, "gpt-5.5-pro")
}
