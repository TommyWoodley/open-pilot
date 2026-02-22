package validation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestREADMEExists verifies the README file exists at the expected path.
func TestREADMEExists(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	if _, err := os.Stat(readmePath); err != nil {
		t.Fatalf("README.md does not exist at %s: %v", readmePath, err)
	}
}

// TestREADMENotEmpty verifies the README file is not empty.
func TestREADMENotEmpty(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	if len(data) == 0 {
		t.Error("README.md is empty")
	}

	content := string(data)
	if strings.TrimSpace(content) == "" {
		t.Error("README.md contains only whitespace")
	}
}

// TestREADMEHasRequiredSections verifies the README contains all required sections.
func TestREADMEHasRequiredSections(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := string(data)

	requiredSections := []string{
		"# open-pilot",
		"## Prerequisites",
		"## Run",
		"## Build",
		"## Local CI checks",
		"## GitHub Actions CI",
		"## Slash commands",
		"## Built-in hooks",
		"## Wrapper protocol",
		"## Codex behavior",
	}

	for _, section := range requiredSections {
		if !strings.Contains(content, section) {
			t.Errorf("README.md missing required section: %s", section)
		}
	}
}

// TestREADMEPrerequisitesSection verifies prerequisites are documented.
func TestREADMEPrerequisitesSection(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := string(data)

	prerequisites := []string{
		"Go 1.24+",
		"codex",
		"PATH",
	}

	for _, prereq := range prerequisites {
		if !strings.Contains(content, prereq) {
			t.Errorf("README.md prerequisites section should mention: %s", prereq)
		}
	}
}

// TestREADMELocalCIChecksMatchCIWorkflow verifies documented commands match CI.
func TestREADMELocalCIChecksMatchCIWorkflow(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := string(data)

	// Verify the README documents the same commands as CI
	expectedCommands := []string{
		"go test ./...",
		"go vet ./...",
	}

	for _, cmd := range expectedCommands {
		if !strings.Contains(content, cmd) {
			t.Errorf("README.md should document CI command: %s", cmd)
		}
	}

	// Verify Local CI checks section exists and contains commands
	if !strings.Contains(content, "## Local CI checks") {
		t.Error("README.md missing 'Local CI checks' section")
	}

	// Find the Local CI checks section
	lines := strings.Split(content, "\n")
	inLocalCISection := false
	foundGoTest := false
	foundGoVet := false

	for _, line := range lines {
		if strings.Contains(line, "## Local CI checks") {
			inLocalCISection = true
			continue
		}
		if inLocalCISection && strings.HasPrefix(line, "## ") {
			// Moved to next section
			break
		}
		if inLocalCISection {
			if strings.Contains(line, "go test ./...") {
				foundGoTest = true
			}
			if strings.Contains(line, "go vet ./...") {
				foundGoVet = true
			}
		}
	}

	if !foundGoTest {
		t.Error("Local CI checks section should contain 'go test ./...'")
	}
	if !foundGoVet {
		t.Error("Local CI checks section should contain 'go vet ./...'")
	}
}

// TestREADMEGitHubActionsCISection verifies the GitHub Actions CI section.
func TestREADMEGitHubActionsCISection(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := string(data)

	// Verify GitHub Actions CI section exists
	if !strings.Contains(content, "## GitHub Actions CI") {
		t.Error("README.md missing 'GitHub Actions CI' section")
	}

	// Find the GitHub Actions CI section
	lines := strings.Split(content, "\n")
	inGitHubCISection := false
	foundMasterBranch := false
	foundChecks := false

	for _, line := range lines {
		if strings.Contains(line, "## GitHub Actions CI") {
			inGitHubCISection = true
			continue
		}
		if inGitHubCISection && strings.HasPrefix(line, "## ") {
			// Moved to next section
			break
		}
		if inGitHubCISection {
			if strings.Contains(line, "master") {
				foundMasterBranch = true
			}
			if strings.Contains(line, "Checks executed:") || strings.Contains(line, "checks") {
				foundChecks = true
			}
		}
	}

	if !foundMasterBranch {
		t.Error("GitHub Actions CI section should mention master branch")
	}
	if !foundChecks {
		t.Error("GitHub Actions CI section should document checks executed")
	}
}

// TestREADMERunSection verifies the Run section contains valid command.
func TestREADMERunSection(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := string(data)

	// Verify Run section exists
	if !strings.Contains(content, "## Run") {
		t.Error("README.md missing 'Run' section")
		return
	}

	// Verify it contains go run command
	if !strings.Contains(content, "go run") {
		t.Error("Run section should contain 'go run' command")
	}

	// Verify it references the main package
	if !strings.Contains(content, "cmd/open-pilot") {
		t.Error("Run section should reference cmd/open-pilot")
	}
}

// TestREADMEBuildSection verifies the Build section contains valid command.
func TestREADMEBuildSection(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := string(data)

	// Verify Build section exists
	if !strings.Contains(content, "## Build") {
		t.Error("README.md missing 'Build' section")
		return
	}

	// Verify it contains go build command
	if !strings.Contains(content, "go build") {
		t.Error("Build section should contain 'go build' command")
	}

	// Verify it references the main package
	if !strings.Contains(content, "cmd/open-pilot") {
		t.Error("Build section should reference cmd/open-pilot")
	}
}

// TestREADMESlashCommandsSection verifies slash commands are documented.
func TestREADMESlashCommandsSection(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := string(data)

	// Verify Slash commands section exists
	if !strings.Contains(content, "## Slash commands") {
		t.Error("README.md missing 'Slash commands' section")
		return
	}

	// Verify some key commands are documented
	expectedCommands := []string{
		"/session new",
		"/session list",
		"/session use",
		"/provider use",
		"/help",
	}

	for _, cmd := range expectedCommands {
		if !strings.Contains(content, cmd) {
			t.Errorf("Slash commands section should document: %s", cmd)
		}
	}
}

// TestREADMEBuiltinHooksSection verifies built-in hooks are documented.
func TestREADMEBuiltinHooksSection(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := string(data)

	// Verify Built-in hooks section exists
	if !strings.Contains(content, "## Built-in hooks") {
		t.Error("README.md missing 'Built-in hooks' section")
		return
	}

	// Verify hooks concepts are documented
	expectedConcepts := []string{
		"hooks/builtin",
		"yaml",
		"session.started",
		"repo.selected",
		"provider.codex.selected",
	}

	for _, concept := range expectedConcepts {
		if !strings.Contains(content, concept) {
			t.Errorf("Built-in hooks section should mention: %s", concept)
		}
	}
}

// TestREADMECodexBehaviorSection verifies Codex behavior is documented.
func TestREADMECodexBehaviorSection(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := string(data)

	// Verify Codex behavior section exists
	if !strings.Contains(content, "## Codex behavior") {
		t.Error("README.md missing 'Codex behavior' section")
		return
	}

	// Verify key Codex concepts are documented
	expectedConcepts := []string{
		"Codex",
		"adapter",
		"thread",
	}

	for _, concept := range expectedConcepts {
		if !strings.Contains(content, concept) {
			t.Errorf("Codex behavior section should mention: %s", concept)
		}
	}
}

// TestREADMEHasProjectDescription verifies README has a project description.
func TestREADMEHasProjectDescription(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := string(data)

	// Verify project is described
	if !strings.Contains(content, "open-pilot") {
		t.Error("README.md should contain project name 'open-pilot'")
	}

	// Verify it mentions TUI
	if !strings.Contains(content, "TUI") {
		t.Error("README.md should mention that open-pilot is a TUI")
	}

	// Verify it mentions the purpose
	if !strings.Contains(content, "coding-agent") || !strings.Contains(content, "CLI") {
		t.Error("README.md should describe the project's purpose")
	}
}

// TestREADMEMarkdownFormatting verifies basic markdown formatting.
func TestREADMEMarkdownFormatting(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := string(data)

	// Should start with # header
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "# ") {
		t.Error("README.md should start with a top-level heading (# )")
	}

	// Should have code blocks for commands
	if !strings.Contains(content, "```bash") && !strings.Contains(content, "```") {
		t.Error("README.md should use code blocks for command examples")
	}
}

// TestREADMEWrapperProtocolSection verifies wrapper protocol documentation.
func TestREADMEWrapperProtocolSection(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := string(data)

	// Verify Wrapper protocol section exists
	if !strings.Contains(content, "## Wrapper protocol") {
		t.Error("README.md missing 'Wrapper protocol' section")
		return
	}

	// Verify it mentions key concepts
	expectedConcepts := []string{
		"Codex",
		"wrapper",
	}

	for _, concept := range expectedConcepts {
		if !strings.Contains(content, concept) {
			t.Errorf("Wrapper protocol section should mention: %s", concept)
		}
	}
}

// TestREADMEDevelopmentWorkCompleteHook verifies development.work.complete hook is documented.
func TestREADMEDevelopmentWorkCompleteHook(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := string(data)

	// Verify development.work.complete trigger is documented
	if !strings.Contains(content, "development.work.complete") {
		t.Error("README.md should document development.work.complete hook trigger")
	}

	// Verify it mentions the marker pattern
	if !strings.Contains(content, "DEVELOPMENT_WORK_COMPLETE") {
		t.Error("README.md should document DEVELOPMENT_WORK_COMPLETE marker")
	}
}

// TestREADMEConsistentSectionDepth verifies consistent heading depth.
func TestREADMEConsistentSectionDepth(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	hasH1 := false
	hasH2 := false

	for _, line := range lines {
		if strings.HasPrefix(line, "# ") && !strings.HasPrefix(line, "## ") {
			hasH1 = true
		}
		if strings.HasPrefix(line, "## ") {
			hasH2 = true
		}
	}

	if !hasH1 {
		t.Error("README.md should have at least one H1 heading")
	}
	if !hasH2 {
		t.Error("README.md should have at least one H2 heading for sections")
	}
}

// TestREADMEInstallBuiltinSkillsHook verifies install-builtin-skills hook is documented.
func TestREADMEInstallBuiltinSkillsHook(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	content := string(data)

	// Verify install-builtin-skills-on-codex-select is documented
	if !strings.Contains(content, "install-builtin-skills-on-codex-select") {
		t.Error("README.md should document install-builtin-skills-on-codex-select hook")
	}

	// Verify it mentions the skills source
	if !strings.Contains(content, "pilot-superpowers") {
		t.Error("README.md should document pilot-superpowers skills source")
	}
}