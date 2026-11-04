package main

import "strings"

// Codex expects reasoning presets to contain effort and description fields.
type reasoningEffortPreset struct {
	Effort      string `json:"effort"`
	Description string `json:"description"`
}

type upstreamCapabilities struct {
	Reasoning   bool     `json:"reasoning"`
	EffortTiers []string `json:"effort_tiers"`
}

var reasoningDescriptions = map[string]string{
	"none":       "No reasoning",
	"minimal":    "Minimal reasoning",
	"low":        "Low reasoning",
	"medium":     "Medium reasoning",
	"high":       "High reasoning",
	"xhigh":      "Extra-high reasoning",
	"max":        "Maximum reasoning",
	"ultra":      "Ultra reasoning",
	"persistent": "Persistent reasoning",
}

// Map only explicitly advertised reasoning tiers. Some upstream entries expose
// effort_tiers for non-reasoning models, and others provide no tier details.
func modelReasoningLevels(slug string, cap upstreamCapabilities) (*string, []reasoningEffortPreset) {
	levels := make([]reasoningEffortPreset, 0, len(cap.EffortTiers))
	if !cap.Reasoning {
		return nil, levels
	}
	seen := make(map[string]bool, len(cap.EffortTiers))
	for _, raw := range cap.EffortTiers {
		effort := strings.TrimSpace(raw)
		if effort == "" || len(effort) > 64 || strings.ContainsAny(effort, "\r\n\t") || seen[effort] {
			continue
		}
		seen[effort] = true
		label := reasoningDescriptions[effort]
		if label == "" {
			// Modern Codex supports custom effort strings; preserve provider metadata.
			label = "Reasoning effort: " + effort
		}
		levels = append(levels, reasoningEffortPreset{Effort: effort, Description: label})
	}
	if len(levels) == 0 {
		return nil, levels
	}
	// Many OmniRoute model IDs are effort-fixed aliases (-low, -high, etc.).
	// Choose that variant's tier when it is advertised; otherwise prefer medium.
	selected := ""
	for _, option := range levels {
		if strings.HasSuffix(slug, "-"+option.Effort) {
			selected = option.Effort
			break
		}
	}
	if selected == "" {
		for _, preference := range []string{"medium", "low"} {
			if seen[preference] {
				selected = preference
				break
			}
		}
	}
	if selected == "" {
		selected = levels[0].Effort
	}
	return &selected, levels
}
