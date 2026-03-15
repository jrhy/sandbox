package readmegen

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

const (
	ReadmeWalkthroughStart = "<!-- walkthrough:start -->"
	ReadmeWalkthroughEnd   = "<!-- walkthrough:end -->"
)

type Transcript struct {
	GeneratedAt   time.Time
	StoragePaths  []string
	EmbedModel    string
	MetadataModel string
	Steps         []Step
}

type Step struct {
	Title    string
	Command  string
	Response string
}

func RenderWalkthrough(transcript Transcript) (string, error) {
	if len(transcript.StoragePaths) == 0 {
		return "", fmt.Errorf("storage paths are required")
	}
	if len(transcript.Steps) == 0 {
		return "", fmt.Errorf("walkthrough steps are required")
	}

	var out bytes.Buffer
	out.WriteString("## Walkthrough\n\n")
	out.WriteString("This section is generated from `./scripts/walkthrough.sh`.\n\n")
	out.WriteString("### Persistent data\n\n")
	out.WriteString("Persistent data lives in:\n")
	for _, path := range transcript.StoragePaths {
		out.WriteString(fmt.Sprintf("- `%s`\n", path))
	}
	out.WriteString("\n")
	if strings.TrimSpace(transcript.EmbedModel) != "" || strings.TrimSpace(transcript.MetadataModel) != "" {
		out.WriteString("### Models used in this walkthrough\n\n")
		if strings.TrimSpace(transcript.EmbedModel) != "" {
			out.WriteString(fmt.Sprintf("- Embedding model: `%s`\n", transcript.EmbedModel))
		}
		if strings.TrimSpace(transcript.MetadataModel) != "" {
			out.WriteString(fmt.Sprintf("- Metadata model: `%s`\n", transcript.MetadataModel))
		}
		out.WriteString("\n")
	}
	if !transcript.GeneratedAt.IsZero() {
		out.WriteString(fmt.Sprintf("_Generated at %s._\n\n", transcript.GeneratedAt.UTC().Format(time.RFC3339)))
	}
	for _, step := range transcript.Steps {
		out.WriteString(fmt.Sprintf("### %s\n\n", step.Title))
		out.WriteString("```bash\n")
		out.WriteString(normalizeBlockText(step.Command))
		out.WriteString("\n```\n\n")
		out.WriteString("```text\n")
		out.WriteString(normalizeBlockText(step.Response))
		out.WriteString("\n```\n\n")
	}

	return strings.TrimRight(out.String(), "\n") + "\n", nil
}

func UpdateREADME(existing, walkthrough string) (string, error) {
	start := strings.Index(existing, ReadmeWalkthroughStart)
	end := strings.Index(existing, ReadmeWalkthroughEnd)
	if start == -1 || end == -1 || end < start {
		return "", fmt.Errorf("README walkthrough markers are required")
	}
	start += len(ReadmeWalkthroughStart)
	var out strings.Builder
	out.WriteString(existing[:start])
	out.WriteString("\n\n")
	out.WriteString(strings.TrimSpace(walkthrough))
	out.WriteString("\n\n")
	out.WriteString(existing[end:])
	return out.String(), nil
}

func normalizeBlockText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.TrimSpace(value)
}
