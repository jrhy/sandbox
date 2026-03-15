package readmegen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"
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
	out.WriteString("This section is generated from `./scripts/walkthrough.sh` using real interactions. For readability, long `curl` commands are wrapped and JSON bodies are pretty-printed.\n\n")
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
		out.WriteString(renderCommand(step.Command))
		out.WriteString("\n```\n\n")
		out.WriteString("```text\n")
		out.WriteString(renderResponse(step.Response))
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

func renderCommand(value string) string {
	value = normalizeBlockText(value)
	if !strings.HasPrefix(value, "curl ") {
		return value
	}

	tokens, ok := splitShellWords(value)
	if !ok || len(tokens) == 0 || tokens[0] != "curl" {
		return value
	}

	parts := []string{"curl"}
	for index := 1; index < len(tokens); index++ {
		token := tokens[index]
		if isFlagWithValue(token) && index+1 < len(tokens) {
			argument := tokens[index+1]
			if token == "--data" || token == "-d" || token == "--data-binary" {
				parts = append(parts, token+" "+formatCommandJSON(argument))
			} else {
				parts = append(parts, token+" "+quoteShellToken(argument))
			}
			index++
			continue
		}
		parts = append(parts, quoteShellToken(token))
	}

	return strings.Join(parts, " \\\n  ")
}

func renderResponse(value string) string {
	value = normalizeBlockText(value)
	if value == "" {
		return value
	}

	parts := strings.SplitN(value, "\n\n", 2)
	if len(parts) == 1 {
		if pretty, ok := prettyJSON(parts[0]); ok {
			return pretty
		}
		return value
	}

	headers := strings.TrimSpace(parts[0])
	body := strings.TrimSpace(parts[1])
	if pretty, ok := prettyJSON(body); ok {
		return headers + "\n\n" + pretty
	}
	return value
}

func prettyJSON(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || !json.Valid([]byte(trimmed)) {
		return "", false
	}

	var out bytes.Buffer
	if err := json.Indent(&out, []byte(trimmed), "", "  "); err != nil {
		return "", false
	}
	return out.String(), true
}

func formatCommandJSON(value string) string {
	if pretty, ok := prettyJSON(value); ok {
		return singleQuote(pretty)
	}
	return quoteShellToken(value)
}

func isFlagWithValue(token string) bool {
	switch token {
	case "-b", "-c", "-H", "-X", "--data", "-d", "--data-binary":
		return true
	default:
		return false
	}
}

func splitShellWords(value string) ([]string, bool) {
	tokens := make([]string, 0)
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false

	for _, r := range value {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\' && !inSingleQuote:
			escaped = true
		case r == '\'' && !inDoubleQuote:
			inSingleQuote = !inSingleQuote
		case r == '"' && !inSingleQuote:
			inDoubleQuote = !inDoubleQuote
		case unicode.IsSpace(r) && !inSingleQuote && !inDoubleQuote:
			if current.Len() == 0 {
				continue
			}
			tokens = append(tokens, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}

	if escaped || inSingleQuote || inDoubleQuote {
		return nil, false
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens, true
}

func quoteShellToken(value string) string {
	if value == "" {
		return "''"
	}
	if isSafeShellToken(value) {
		return value
	}
	return singleQuote(value)
}

func isSafeShellToken(value string) bool {
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		switch r {
		case '-', '_', '.', '/', ':', '@':
			continue
		default:
			return false
		}
	}
	return true
}

func singleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
