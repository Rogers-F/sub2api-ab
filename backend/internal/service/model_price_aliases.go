package service

import "strings"

const (
	claudeJupiterV1PModel     = "claude-jupiter-v1-p"
	claudeOpus47PricingTarget = "claude-opus-4-7"
)

func pricingAliasForModel(model string) string {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case claudeJupiterV1PModel:
		return claudeOpus47PricingTarget
	default:
		return ""
	}
}
