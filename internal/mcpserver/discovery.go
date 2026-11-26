package mcpserver

import "strings"

// ToolSearchInput is a compact discovery query over the registered catalog.
type ToolSearchInput struct {
	Query string `json:"query" jsonschema:"keyword matched against tool name, title, category, capability, and description"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum results (1-25, default 10)"`
}

// ToolSearchOutput returns catalog metadata only, never large JSON schemas.
type ToolSearchOutput struct {
	Query string     `json:"query"`
	Count int        `json:"count"`
	Tools []ToolSpec `json:"tools"`
}

func searchToolSpecs(specs []ToolSpec, query string, limit int) ToolSearchOutput {
	query = strings.ToLower(strings.TrimSpace(query))
	if limit <= 0 || limit > 25 {
		limit = 10
	}
	out := ToolSearchOutput{Query: query, Tools: []ToolSpec{}}
	if query == "" {
		return out
	}
	for _, spec := range specs {
		haystack := strings.ToLower(spec.Name + " " + spec.Title + " " + spec.Category + " " + spec.Capability + " " + spec.Description)
		if !strings.Contains(haystack, query) {
			continue
		}
		out.Tools = append(out.Tools, spec)
		if len(out.Tools) >= limit {
			break
		}
	}
	out.Count = len(out.Tools)
	return out
}
