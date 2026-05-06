package openai

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultModels_ContainsGPT55Models(t *testing.T) {
	ids := DefaultModelIDs()

	require.Contains(t, ids, "gpt-5.5")
	require.Contains(t, ids, "gpt-5.5-pro")
}
