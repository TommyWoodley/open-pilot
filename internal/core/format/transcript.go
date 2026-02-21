package format

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/thwoodle/open-pilot/internal/domain"
)

type RenderedMessage struct {
	Prefix string
	Body   string
}

type Styles struct {
	UserPrefix           func(string) string
	AgentPrefix          func(string) string
	AgentMeta            func(string) string
	SystemPrefix         func(string) string
	Heading              func(string) string
	List                 func(string) string
	Quote                func(string) string
	Link                 func(label, url string) string
	InlineCode           func(string) string
	Bold                 func(string) string
	Italic               func(string) string
	Strike               func(string) string
	CodeBlock            func(lang, text string) string
	StreamingPlaceholder func() string
	StreamingSuffix      func() string
}

func FormatMessageForTranscript(msg domain.Message, styles Styles) RenderedMessage {
	prefix := "[pilot]"
	prefixRender := styles.SystemPrefix
	switch msg.Role {
	case domain.RoleUser:
		prefix = "[you]"
		prefixRender = styles.UserPrefix
	case domain.RoleAssistant:
		prefix = "[agent]"
		prefixRender = styles.AgentPrefix
	}
	if prefixRender != nil {
		prefix = prefixRender(prefix)
	}

	blocks := ParseMarkdownBlocks(msg.Content, msg.Streaming)
	lines := make([]string, 0, len(blocks)+1)
	for _, block := range blocks {
		switch block.Kind {
		case BlockParagraph:
			for _, l := range strings.Split(block.Text, "\n") {
				if isTranscriptSectionTag(strings.TrimSpace(l)) {
					lines = append(lines, l)
					continue
				}
				lines = append(lines, RenderInline(l, InlineStyles{
					Code:   styles.InlineCode,
					Link:   styles.Link,
					Bold:   styles.Bold,
					Italic: styles.Italic,
					Strike: styles.Strike,
				}))
			}
		case BlockHeading:
			txt := RenderInline(block.Text, InlineStyles{
				Code:   styles.InlineCode,
				Link:   styles.Link,
				Bold:   styles.Bold,
				Italic: styles.Italic,
				Strike: styles.Strike,
			})
			if styles.Heading != nil {
				txt = styles.Heading(txt)
			}
			lines = append(lines, txt)
		case BlockList:
			txt := RenderInline(block.Text, InlineStyles{
				Code:   styles.InlineCode,
				Link:   styles.Link,
				Bold:   styles.Bold,
				Italic: styles.Italic,
				Strike: styles.Strike,
			})
			if styles.List != nil {
				txt = styles.List(txt)
			}
			lines = append(lines, txt)
		case BlockQuote:
			txt := RenderInline(block.Text, InlineStyles{
				Code:   styles.InlineCode,
				Link:   styles.Link,
				Bold:   styles.Bold,
				Italic: styles.Italic,
				Strike: styles.Strike,
			})
			if styles.Quote != nil {
				txt = styles.Quote(txt)
			}
			lines = append(lines, txt)
		case BlockCode:
			if styles.CodeBlock != nil {
				lines = append(lines, styles.CodeBlock(block.Lang, block.Text))
			} else {
				lines = append(lines, block.Text)
			}
		case BlockBlank:
			lines = append(lines, "")
		}
	}

	body := strings.Join(lines, "\n")
	if msg.Streaming {
		if body == "" {
			if styles.StreamingPlaceholder != nil {
				body = styles.StreamingPlaceholder()
			} else {
				body = "..."
			}
		} else {
			if styles.StreamingSuffix != nil {
				body += "\n" + styles.StreamingSuffix()
			} else {
				body += "\n..."
			}
		}
	}

	return RenderedMessage{Prefix: prefix, Body: body}
}

func BuildTranscriptLines(messages []domain.Message, styles Styles) []string {
	if len(messages) == 0 {
		return nil
	}
	lines := make([]string, 0, len(messages))
	for _, msg := range messages {
		formatted := FormatMessageForTranscript(msg, styles)
		if formatted.Body == "" {
			lines = append(lines, formatted.Prefix)
		} else {
			bodyLines := strings.Split(formatted.Body, "\n")
			for i := range bodyLines {
				bodyLines[i] = normalizeTranscriptSectionTag(bodyLines[i])
			}

			metaMask := classifyAgentMetaLines(msg, bodyLines)
			continuationIndent := strings.Repeat(" ", visibleTextWidth(formatted.Prefix)+1)
			hasPrefixedLine := false
			for i := 0; i < len(bodyLines); i++ {
				line := bodyLines[i]
				if isPilotDividerToken(line) {
					lines = append(lines, line)
					continue
				}
				if metaMask[i] && styles.AgentMeta != nil {
					line = styles.AgentMeta(line)
				}
				if !hasPrefixedLine {
					lines = append(lines, formatted.Prefix+" "+line)
					hasPrefixedLine = true
				} else {
					lines = append(lines, continuationIndent+line)
				}
			}
		}
		lines = append(lines, "")
	}
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func isPilotDividerToken(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "[[pilot-divider:") && strings.HasSuffix(trimmed, "]]")
}

func normalizeTranscriptSectionTag(line string) string {
	trimmed := strings.TrimSpace(line)
	if matches := sectionStartTagRegex.FindStringSubmatch(trimmed); len(matches) == 2 {
		return "[[pilot-divider:" + formatTranscriptSectionTitle(matches[1]) + "]]"
	}
	if matches := sectionEndTagRegex.FindStringSubmatch(trimmed); len(matches) == 2 {
		return "[[pilot-divider:]]"
	}
	if developmentWorkCompleteTagRegex.MatchString(trimmed) || developmentWorkCompleteAngleTagRegex.MatchString(trimmed) {
		return "[[pilot-divider:Development Work Complete]]"
	}
	return line
}

func formatTranscriptSectionTitle(raw string) string {
	parts := strings.Split(raw, "_")
	for i, part := range parts {
		part = strings.ToLower(part)
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func isTranscriptSectionTag(line string) bool {
	return sectionStartTagRegex.MatchString(line) ||
		sectionEndTagRegex.MatchString(line) ||
		developmentWorkCompleteTagRegex.MatchString(strings.TrimSpace(line)) ||
		developmentWorkCompleteAngleTagRegex.MatchString(strings.TrimSpace(line))
}

func classifyAgentMetaLines(msg domain.Message, bodyLines []string) []bool {
	meta := make([]bool, len(bodyLines))
	if msg.Role != domain.RoleAssistant || len(bodyLines) == 0 {
		return meta
	}
	inCommandOutput := false
	allowErrorLine := false
	for i, line := range bodyLines {
		trimmed := strings.TrimLeft(line, " ")
		switch {
		case strings.HasPrefix(trimmed, "[agent-thought]"),
			runningCommandSummaryRegex.MatchString(trimmed),
			completedCommandSummaryRegex.MatchString(trimmed),
			exploredCommandSummaryRegex.MatchString(trimmed),
			strings.HasPrefix(trimmed, "Running command:"),
			strings.HasPrefix(trimmed, "Command completed"),
			strings.HasPrefix(trimmed, "Command failed"),
			strings.HasPrefix(trimmed, "Command output:"):
			meta[i] = true
			inCommandOutput = strings.HasPrefix(trimmed, "Command output:")
			allowErrorLine = strings.HasPrefix(trimmed, "Command failed") || strings.Contains(trimmed, "(failed, exit=")
		case allowErrorLine && strings.HasPrefix(trimmed, "Error:"):
			meta[i] = true
			inCommandOutput = false
			allowErrorLine = false
		case inCommandOutput:
			if strings.TrimSpace(trimmed) == "" {
				meta[i] = true
				inCommandOutput = false
				allowErrorLine = false
				continue
			}
			meta[i] = true
			allowErrorLine = false
		default:
			inCommandOutput = false
			allowErrorLine = false
		}
	}
	return meta
}

var sgrANSIRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)
var runningCommandSummaryRegex = regexp.MustCompile(`^Running .+ \.\.\.$`)
var completedCommandSummaryRegex = regexp.MustCompile(`^Ran .+ for .+`)
var exploredCommandSummaryRegex = regexp.MustCompile(`^Explored(?: \d+ commands)? for .+`)
var sectionStartTagRegex = regexp.MustCompile(`^<([A-Z0-9_]+)_START>$`)
var sectionEndTagRegex = regexp.MustCompile(`^<([A-Z0-9_]+)_END>$`)
var developmentWorkCompleteTagRegex = regexp.MustCompile(`^\[(?:<)?DEVELOPMENT_WORK_COMPLETE(?:>)?\]$`)
var developmentWorkCompleteAngleTagRegex = regexp.MustCompile(`^<DEVELOPMENT_WORK_COMPLETE>$`)

func visibleTextWidth(s string) int {
	plain := sgrANSIRegex.ReplaceAllString(s, "")
	return utf8.RuneCountInString(plain)
}
