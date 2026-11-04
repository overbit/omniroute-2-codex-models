package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestModelSpecificReasoningLevels(t *testing.T) {
	cases := []struct {
		name, id      string
		caps          upstreamCapabilities
		defaultEffort string
		efforts       []string
	}{
		{"reasoning with all tiers", "agy/claude-sonnet-4-6", upstreamCapabilities{Reasoning: true, EffortTiers: []string{"none", "low", "medium", "high", "xhigh", "max"}}, "medium", []string{"none", "low", "medium", "high", "xhigh", "max"}},
		{"low variant", "agy/claude-sonnet-4-6-low", upstreamCapabilities{Reasoning: true, EffortTiers: []string{"none", "low", "medium", "high", "xhigh", "max"}}, "low", []string{"none", "low", "medium", "high", "xhigh", "max"}},
		{"high variant", "agy/claude-sonnet-4-6-high", upstreamCapabilities{Reasoning: true, EffortTiers: []string{"none", "low", "medium", "high", "xhigh", "max"}}, "high", []string{"none", "low", "medium", "high", "xhigh", "max"}},
		{"ultra model", "codex/gpt-6-astra", upstreamCapabilities{Reasoning: true, EffortTiers: []string{"low", "medium", "high", "xhigh", "max", "ultra"}}, "medium", []string{"low", "medium", "high", "xhigh", "max", "ultra"}},
		{"max variant", "cx/gpt-6-astra-max", upstreamCapabilities{Reasoning: true, EffortTiers: []string{"low", "medium", "high", "xhigh", "max", "ultra"}}, "max", []string{"low", "medium", "high", "xhigh", "max", "ultra"}},
		{"sparse levels", "kmca/k3", upstreamCapabilities{Reasoning: true, EffortTiers: []string{"low", "high", "max"}}, "low", []string{"low", "high", "max"}},
		{"reasoning false despite tiers", "vag/meta/muse-image-1.0", upstreamCapabilities{Reasoning: false, EffortTiers: []string{"minimal", "low", "medium", "high", "xhigh"}}, "", []string{}},
		{"reasoning false", "agy/gemini-3-flash", upstreamCapabilities{Reasoning: false}, "", []string{}},
		{"reasoning true but no tiers", "sprint-local", upstreamCapabilities{Reasoning: true}, "medium", []string{"none", "medium", "high", "xhigh"}},
		{"reasoning true no tiers high variant", "custom/model-high", upstreamCapabilities{Reasoning: true}, "high", []string{"none", "medium", "high", "xhigh"}},
		{"reasoning true empty tiers", "opencode-zen/glm-5.3-flash-free", upstreamCapabilities{Reasoning: true, EffortTiers: []string{}}, "medium", []string{"none", "medium", "high", "xhigh"}},
		{"minimal levels", "custom/reasoning-minimal", upstreamCapabilities{Reasoning: true, EffortTiers: []string{"minimal"}}, "minimal", []string{"minimal"}},
		{"deduplicate invalid or whitespace levels", "model", upstreamCapabilities{Reasoning: true, EffortTiers: []string{"low", "low", "", " medium ", "high"}}, "medium", []string{"low", "medium", "high"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defaultLevel, options := modelReasoningLevels(c.id, c.caps)
			if c.defaultEffort == "" {
				if defaultLevel != nil {
					t.Fatalf("unexpected default: %q", *defaultLevel)
				}
			} else if defaultLevel == nil || *defaultLevel != c.defaultEffort {
				t.Fatalf("default = %v, want %s", defaultLevel, c.defaultEffort)
			}
			got := make([]string, 0, len(options))
			for _, option := range options {
				got = append(got, option.Effort)
				if option.Description == "" {
					t.Errorf("missing description for %q", option.Effort)
				}
				data, err := json.Marshal(option)
				if err != nil {
					t.Fatal(err)
				}
				var shape map[string]string
				if err := json.Unmarshal(data, &shape); err != nil {
					t.Fatal(err)
				}
				if shape["effort"] != option.Effort || shape["description"] != option.Description {
					t.Errorf("unexpected Codex reasoning preset shape: %s", data)
				}
			}
			if !reflect.DeepEqual(got, c.efforts) {
				t.Fatalf("efforts %q, want %q", got, c.efforts)
			}
		})
	}
}
