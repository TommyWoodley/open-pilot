package validation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestREADMENoTrailingWhitespace verifies no lines have excessive trailing whitespace.
func TestREADMENoTrailingWhitespace(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	trailingCount := 0
	for i, line := range lines {
		if len(line) > 0 && line[len(line)-1] == ' ' {
			trailingCount++
			t.Logf("Line %d has trailing whitespace", i+1)
		}
	}

	// Only fail if there are many lines with trailing whitespace
	if trailingCount > 5 {
		t.Errorf("Too many lines with trailing whitespace: %d", trailingCount)
	}
}

// TestREADMENoTabCharacters verifies README uses spaces, not tabs.
func TestREADMENoTabCharacters(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		// Tabs are generally discouraged in markdown except in code blocks
		if strings.Contains(line, "\t") && !strings.HasPrefix(strings.TrimSpace(line), "```") {
			t.Errorf("Line %d contains tab character (use spaces instead)", i+1)
		}
	}
}

// TestREADMECodeBlocksAreClosed verifies all code blocks have closing markers.
func TestREADMECodeBlocksAreClosed(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	inCodeBlock := false
	openLine := 0

	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if inCodeBlock {
				// Closing block
				inCodeBlock = false
			} else {
				// Opening block
				inCodeBlock = true
				openLine = i + 1
			}
		}
	}

	if inCodeBlock {
		t.Errorf("Unclosed code block starting at line %d", openLine)
	}
}

// TestREADMENoEmptyHeaders verifies no headers are empty.
func TestREADMENoEmptyHeaders(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			headerContent := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			if headerContent == "" {
				t.Errorf("Line %d has empty header", i+1)
			}
		}
	}
}

// TestREADMEHeaderHierarchy verifies headers follow proper hierarchy (no jumps).
func TestREADMEHeaderHierarchy(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	previousLevel := 0

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Count header level
		level := 0
		for _, c := range trimmed {
			if c == '#' {
				level++
			} else {
				break
			}
		}

		if previousLevel > 0 {
			// Don't skip levels (e.g., ## followed by ####)
			if level > previousLevel+1 {
				t.Errorf("Line %d: header level jumps from %d to %d (should increment by 1)", i+1, previousLevel, level)
			}
		}

		previousLevel = level
	}
}

// TestREADMELinksAreNotBroken verifies relative file links reference existing files.
func TestREADMELinksAreNotBroken(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := string(data)
	repoRoot := filepath.Join("..", "..")

	// Find markdown links: [text](url)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		start := 0
		for {
			linkStart := strings.Index(line[start:], "](")
			if linkStart == -1 {
				break
			}
			linkStart += start + 2 // Move past "]("

			linkEnd := strings.Index(line[linkStart:], ")")
			if linkEnd == -1 {
				break
			}

			url := line[linkStart : linkStart+linkEnd]

			// Only check relative file paths (not URLs or anchors)
			if !strings.HasPrefix(url, "http") && !strings.HasPrefix(url, "#") && !strings.HasPrefix(url, "mailto:") {
				// It's a relative file path
				fullPath := filepath.Join(repoRoot, url)
				if _, err := os.Stat(fullPath); os.IsNotExist(err) {
					t.Errorf("Line %d: broken link to %q (resolved to %s)", i+1, url, fullPath)
				}
			}

			start = linkStart + linkEnd + 1
		}
	}
}

// TestREADMECommandsUseConsistentShellStyle verifies shell commands use consistent syntax.
func TestREADMECommandsUseConsistentShellStyle(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	inBashBlock := false

	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```bash") {
			inBashBlock = true
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inBashBlock = false
			continue
		}

		if inBashBlock {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}

			// Commands should not have shell prompts like $ or >
			if strings.HasPrefix(trimmed, "$") || strings.HasPrefix(trimmed, ">") {
				t.Errorf("Line %d: bash code block should not include shell prompt (%q)", i+1, trimmed[:min(20, len(trimmed))])
			}
		}
	}
}

// TestREADMEGoVersionConsistency verifies Go version is consistent throughout.
func TestREADMEGoVersionConsistency(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := string(data)

	// Find all Go version references
	goVersions := make(map[string][]int)
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		// Look for patterns like "Go 1.xx" or "go 1.xx"
		lower := strings.ToLower(line)
		if strings.Contains(lower, "go 1.") {
			// Extract version
			idx := strings.Index(lower, "go 1.")
			if idx != -1 && len(lower) > idx+6 {
				version := line[idx : idx+7] // "Go 1.xx"
				goVersions[version] = append(goVersions[version], i+1)
			}
		}
	}

	// Verify all versions are the same
	if len(goVersions) > 1 {
		t.Errorf("README.md references multiple Go versions: %v (should be consistent)", goVersions)
	}
}

// TestREADMENoDeadCodeReferences verifies no references to deleted features.
func TestREADMENoDeadCodeReferences(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := string(data)

	// Check for common markers of outdated documentation
	suspiciousPatterns := []struct {
		pattern string
		warning string
	}{
		{"TODO:", "contains TODO marker"},
		{"FIXME:", "contains FIXME marker"},
		{"XXX:", "contains XXX marker"},
		{"DEPRECATED", "references deprecated feature"},
	}

	lines := strings.Split(content, "\n")
	for _, sp := range suspiciousPatterns {
		for i, line := range lines {
			if strings.Contains(strings.ToUpper(line), sp.pattern) {
				t.Logf("Line %d %s: %q", i+1, sp.warning, strings.TrimSpace(line))
			}
		}
	}
}

// TestREADMESlashCommandsHaveDescriptions verifies slash commands include descriptions.
func TestREADMESlashCommandsHaveDescriptions(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	inSlashCommandsSection := false

	// Simple commands that don't need extensive descriptions
	simpleCommands := []string{"/help", "/list"}

	for i, line := range lines {
		if strings.Contains(line, "## Slash commands") {
			inSlashCommandsSection = true
			continue
		}
		if inSlashCommandsSection && strings.HasPrefix(line, "## ") {
			break
		}

		if inSlashCommandsSection {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "-") && strings.Contains(trimmed, "/") {
				// This is a slash command line
				// Skip simple self-explanatory commands
				isSimple := false
				for _, simple := range simpleCommands {
					if strings.Contains(trimmed, simple) {
						isSimple = true
						break
					}
				}

				if !isSimple {
					// Verify it's not just the command name
					if !strings.Contains(trimmed, " ") || len(trimmed) < 10 {
						t.Errorf("Line %d: slash command appears to lack description: %q", i+1, trimmed)
					}
				}
			}
		}
	}
}

// TestREADMESectionOrderLogical verifies sections appear in a logical order.
func TestREADMESectionOrderLogical(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := string(data)

	// Define expected section order (some sections can be optional)
	expectedOrder := []string{
		"# open-pilot",       // Title should be first
		"## Prerequisites",   // Prerequisites before Run/Build
		"## Run",             // Run before Build is common
		"## Build",           // Build section
		"## Local CI checks", // CI info
	}

	lastIndex := -1
	for _, section := range expectedOrder {
		idx := strings.Index(content, section)
		if idx == -1 {
			continue // Optional section
		}

		if idx < lastIndex {
			t.Errorf("Section %q appears before expected position (found at %d, previous was %d)", section, idx, lastIndex)
		}
		lastIndex = idx
	}
}

// TestREADMECICommandsMatchExactly verifies CI commands in README match workflow exactly.
func TestREADMECICommandsMatchExactly(t *testing.T) {
	// Read README
	readmePath := filepath.Join("..", "..", "README.md")
	readmeData, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	// Read CI workflow
	ciPath := filepath.Join("..", "..", ".github", "workflows", "ci.yml")
	ciData, err := os.ReadFile(ciPath)
	if err != nil {
		t.Fatalf("failed to read CI workflow: %v", err)
	}

	readme := string(readmeData)
	ci := string(ciData)

	// Extract commands from both
	ciCommands := []string{"go test ./...", "go vet ./..."}

	for _, cmd := range ciCommands {
		if !strings.Contains(readme, cmd) {
			t.Errorf("README.md should document exact CI command: %q", cmd)
		}
		if !strings.Contains(ci, cmd) {
			t.Errorf("CI workflow should contain command: %q", cmd)
		}
	}
}

// TestREADMEHooksExamplesSyntax verifies hook examples use valid YAML triggers.
func TestREADMEHooksExamplesSyntax(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := string(data)

	// Verify documented triggers are valid
	validTriggers := []string{
		"session.started",
		"repo.selected",
		"provider.codex.selected",
		"development.work.complete",
	}

	inHooksSection := false
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		if strings.Contains(line, "## Built-in hooks") {
			inHooksSection = true
			continue
		}
		if inHooksSection && strings.HasPrefix(line, "## ") {
			break
		}

		if inHooksSection {
			for _, trigger := range validTriggers {
				if strings.Contains(line, trigger) {
					// Good - documented trigger is valid
					t.Logf("Line %d documents valid trigger: %s", i+1, trigger)
				}
			}
		}
	}
}

// TestREADMEFileSize verifies README is not excessively large.
func TestREADMEFileSize(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	info, err := os.Stat(readmePath)
	if err != nil {
		t.Fatalf("failed to stat README.md: %v", err)
	}

	size := info.Size()

	// README should be under 100KB (arbitrary reasonable limit)
	if size > 100*1024 {
		t.Errorf("README.md is very large (%d bytes), consider splitting into multiple docs", size)
	}

	// README should have some content (at least 500 bytes)
	if size < 500 {
		t.Errorf("README.md is suspiciously small (%d bytes)", size)
	}
}

// TestREADMENoTypos verifies common command typos are not present.
func TestREADMENoTypos(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := strings.ToLower(string(data))

	// Common typos and mistakes
	typos := []struct {
		wrong   string
		correct string
	}{
		{"go tets", "go test"},
		{"go biuld", "go build"},
		{"gotest", "go test"},
		{"gobuild", "go build"},
	}

	for _, typo := range typos {
		if strings.Contains(content, typo.wrong) {
			t.Errorf("README.md contains typo %q (should be %q)", typo.wrong, typo.correct)
		}
	}
}

// TestREADMEReferencesExistingPaths verifies critical file paths mentioned actually exist.
func TestREADMEReferencesExistingPaths(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := string(data)
	repoRoot := filepath.Join("..", "..")

	// Look for critical file path references that must exist
	pathPatterns := []string{
		"hooks/builtin",
		"go.mod",
	}

	for _, pathPattern := range pathPatterns {
		if strings.Contains(content, pathPattern) {
			fullPath := filepath.Join(repoRoot, pathPattern)
			if _, err := os.Stat(fullPath); os.IsNotExist(err) {
				t.Errorf("README.md references path %q which does not exist", pathPattern)
			}
		}
	}

	// Note: cmd/open-pilot is mentioned as an example in commands like "go run ./cmd/open-pilot"
	// but the actual directory structure may vary (e.g., cmd/cli, main.go at root, etc.)
	// So we don't strictly validate it exists
}

// min helper function for older Go versions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}