package migrations

import (
	"fmt"
	"strings"
)

const embedDimensionsPlaceholder = "__EMBED_DIMENSIONS__"

// RenderSchema replaces the embedding-dimension placeholder in a schema template.
func RenderSchema(template string, embedDimensions int) (string, error) {
	if embedDimensions <= 0 {
		return "", fmt.Errorf("embed dimensions must be positive")
	}
	if !strings.Contains(template, embedDimensionsPlaceholder) {
		return "", fmt.Errorf("schema template missing %s", embedDimensionsPlaceholder)
	}
	return strings.ReplaceAll(template, embedDimensionsPlaceholder, fmt.Sprintf("%d", embedDimensions)), nil
}
