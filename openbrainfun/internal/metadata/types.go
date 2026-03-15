package metadata

import "strings"

const defaultSummary = "No summary available."

type Metadata struct {
	Summary  string
	Topics   []string
	Entities []string
}

func Normalize(raw map[string]any) Metadata {
	metadata := Metadata{
		Summary:  defaultSummary,
		Topics:   []string{},
		Entities: []string{},
	}
	if raw == nil {
		return metadata
	}

	if summary, ok := raw["summary"].(string); ok && strings.TrimSpace(summary) != "" {
		metadata.Summary = strings.TrimSpace(summary)
	}
	metadata.Topics = normalizeStringSlice(raw["topics"])
	metadata.Entities = normalizeStringSlice(raw["entities"])
	return metadata
}

func normalizeStringSlice(value any) []string {
	switch typed := value.(type) {
	case nil:
		return []string{}
	case []string:
		return compactStrings(typed)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			asString, ok := item.(string)
			if !ok {
				continue
			}
			trimmed := strings.TrimSpace(asString)
			if trimmed == "" {
				continue
			}
			values = append(values, trimmed)
		}
		return values
	default:
		return []string{}
	}
}

func compactStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}
