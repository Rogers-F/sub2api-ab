package service

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFallbackPricingContainsGemini35Flash(t *testing.T) {
	body, err := os.ReadFile("../../resources/model-pricing/model_prices_and_context_window.json")
	require.NoError(t, err)

	svc := &PricingService{}
	pricingMap, err := svc.parsePricingData(body)
	require.NoError(t, err)

	pricing := pricingMap["gemini-3.5-flash"]
	require.NotNil(t, pricing)
	require.InDelta(t, 1.50e-6, pricing.InputCostPerToken, 1e-12)
	require.InDelta(t, 9.00e-6, pricing.OutputCostPerToken, 1e-12)
	require.InDelta(t, 0.15e-6, pricing.CacheReadInputTokenCost, 1e-12)
	require.InDelta(t, 2.70e-6, pricing.InputCostPerTokenPriority, 1e-12)
	require.InDelta(t, 16.20e-6, pricing.OutputCostPerTokenPriority, 1e-12)
	require.InDelta(t, 0.27e-6, pricing.CacheReadInputTokenCostPriority, 1e-12)
	require.True(t, pricing.SupportsPromptCaching)
	require.True(t, pricing.SupportsServiceTier)
}
