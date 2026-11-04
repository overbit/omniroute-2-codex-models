package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestConvertCatalogReasoningFromOmniRouteSample(t *testing.T) {
	// Representative, non-sensitive entries from an actual OmniRoute model list.
	input := []byte(`{"data":[
      {"id":"agy/claude-sonnet-4-6","capabilities":{"reasoning":true,"thinking":true,"effort_tiers":["none","low","medium","high","xhigh","max"]}},
      {"id":"agy/claude-sonnet-4-6-low","capabilities":{"reasoning":true,"thinking":true,"effort_tiers":["none","low","medium","high","xhigh","max"]}},
      {"id":"codex/gpt-6-astra","capabilities":{"reasoning":true,"effort_tiers":["low","medium","high","xhigh","max","ultra"]}},
      {"id":"cx/gpt-6-astra-max","capabilities":{"reasoning":true,"effort_tiers":["low","medium","high","xhigh","max","ultra"]}},
      {"id":"kmca/k3","capabilities":{"reasoning":true,"effort_tiers":["low","high","max"]}},
      {"id":"vag/meta/muse-image-1.0","capabilities":{"reasoning":false,"thinking":false,"effort_tiers":["minimal","low","medium","high","xhigh"]}},
      {"id":"sprint-local","capabilities":{"reasoning":true}},
      {"id":"custom/model-high","capabilities":{"reasoning":true,"effort_tiers":[]}}
    ]}`)
	output, err := convertCatalog(input)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Models []struct {
			Slug    string                  `json:"slug"`
			Default *string                 `json:"default_reasoning_level"`
			Levels  []reasoningEffortPreset `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		t.Fatal(err)
	}
	expected := map[string]struct {
		defaultEffort string
		efforts       []string
	}{
		"agy/claude-sonnet-4-6":     {"medium", []string{"none", "low", "medium", "high", "xhigh", "max"}},
		"agy/claude-sonnet-4-6-low": {"low", []string{"none", "low", "medium", "high", "xhigh", "max"}},
		"codex/gpt-6-astra":         {"medium", []string{"low", "medium", "high", "xhigh", "max", "ultra"}},
		"cx/gpt-6-astra-max":        {"max", []string{"low", "medium", "high", "xhigh", "max", "ultra"}},
		"kmca/k3":                   {"low", []string{"low", "high", "max"}},
		"vag/meta/muse-image-1.0":   {"", []string{}},
		"sprint-local":              {"medium", []string{"none", "medium", "high", "xhigh"}},
		"custom/model-high":         {"high", []string{"none", "medium", "high", "xhigh"}},
	}
	if len(response.Models) != len(expected) {
		t.Fatalf("translated models %d, want %d", len(response.Models), len(expected))
	}
	for _, model := range response.Models {
		want, ok := expected[model.Slug]
		if !ok {
			t.Fatalf("unexpected model %s", model.Slug)
		}
		if want.defaultEffort == "" {
			if model.Default != nil {
				t.Errorf("%s: got default %s but expected null", model.Slug, *model.Default)
			}
		} else if model.Default == nil || *model.Default != want.defaultEffort {
			t.Errorf("%s: wrong default: %v, want %s", model.Slug, model.Default, want.defaultEffort)
		}
		actual := make([]string, 0, len(model.Levels))
		for _, option := range model.Levels {
			actual = append(actual, option.Effort)
			if option.Description == "" {
				t.Errorf("%s: missing description for %s", model.Slug, option.Effort)
			}
		}
		if !reflect.DeepEqual(actual, want.efforts) {
			t.Errorf("%s: effort presets = %q, want %q", model.Slug, actual, want.efforts)
		}
	}
}
