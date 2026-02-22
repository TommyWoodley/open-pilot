package validation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestCIWorkflowNoSyntaxErrors verifies YAML has no common syntax errors.
func TestCIWorkflowNoSyntaxErrors(t *testing.T) {
	ciPath := filepath.Join("..", "..", ".github", "workflows", "ci.yml")
	data, err := os.ReadFile(ciPath)
	if err != nil {
		t.Fatalf("failed to read CI workflow: %v", err)
	}

	content := string(data)

	// Check for common YAML syntax errors
	if strings.Contains(content, "\t") {
		t.Error("CI workflow should not contain tabs (use spaces for indentation)")
	}

	// Verify no trailing whitespace on key lines
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue // Skip comments
		}
		if len(line) > 0 && line[len(line)-1] == ' ' && strings.TrimSpace(line) != "" {
			t.Errorf("Line %d has trailing whitespace: %q", i+1, line)
		}
	}
}

// TestCIWorkflowNoHardcodedSecrets verifies no secrets are hardcoded.
func TestCIWorkflowNoHardcodedSecrets(t *testing.T) {
	ciPath := filepath.Join("..", "..", ".github", "workflows", "ci.yml")
	data, err := os.ReadFile(ciPath)
	if err != nil {
		t.Fatalf("failed to read CI workflow: %v", err)
	}

	content := strings.ToLower(string(data))

	// Check for potential hardcoded secrets
	suspiciousPatterns := []string{
		"password:",
		"api_key:",
		"secret_key:",
		"access_token:",
		"private_key:",
	}

	for _, pattern := range suspiciousPatterns {
		if strings.Contains(content, pattern) {
			// Make sure it's not just referencing secrets, but actually hardcoding
			lines := strings.Split(string(data), "\n")
			for i, line := range lines {
				if strings.Contains(strings.ToLower(line), pattern) {
					if !strings.Contains(line, "secrets.") && !strings.Contains(line, "env.") {
						t.Errorf("Line %d may contain hardcoded secret: %q", i+1, strings.TrimSpace(line))
					}
				}
			}
		}
	}
}

// TestCIWorkflowTimeoutIsReasonable verifies timeout is neither too short nor too long.
func TestCIWorkflowTimeoutIsReasonable(t *testing.T) {
	ciPath := filepath.Join("..", "..", ".github", "workflows", "ci.yml")
	data, err := os.ReadFile(ciPath)
	if err != nil {
		t.Fatalf("failed to read CI workflow: %v", err)
	}

	var workflow map[string]interface{}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("failed to parse CI workflow: %v", err)
	}

	jobs, ok := workflow["jobs"].(map[string]interface{})
	if !ok {
		t.Fatalf("jobs field is not a map")
	}

	testVet, exists := jobs["test-vet"]
	if !exists {
		t.Fatalf("test-vet job not found")
	}

	testVetJob, ok := testVet.(map[string]interface{})
	if !ok {
		t.Fatalf("test-vet job is not a map")
	}

	timeout, exists := testVetJob["timeout-minutes"]
	if !exists {
		t.Error("test-vet job should have a timeout-minutes setting")
		return
	}

	timeoutInt, ok := timeout.(int)
	if !ok {
		t.Errorf("timeout-minutes should be an integer, got %T", timeout)
		return
	}

	// Verify timeout is reasonable (between 1 and 60 minutes)
	if timeoutInt < 1 {
		t.Errorf("timeout-minutes is too short: %d (should be at least 1)", timeoutInt)
	}
	if timeoutInt > 60 {
		t.Errorf("timeout-minutes is too long: %d (should be at most 60)", timeoutInt)
	}
}

// TestCIWorkflowUsesRecentActionVersions verifies actions use recent versions.
func TestCIWorkflowUsesRecentActionVersions(t *testing.T) {
	ciPath := filepath.Join("..", "..", ".github", "workflows", "ci.yml")
	data, err := os.ReadFile(ciPath)
	if err != nil {
		t.Fatalf("failed to read CI workflow: %v", err)
	}

	var workflow map[string]interface{}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("failed to parse CI workflow: %v", err)
	}

	jobs, ok := workflow["jobs"].(map[string]interface{})
	if !ok {
		t.Fatalf("jobs field is not a map")
	}

	testVet, exists := jobs["test-vet"]
	if !exists {
		t.Fatalf("test-vet job not found")
	}

	testVetJob, ok := testVet.(map[string]interface{})
	if !ok {
		t.Fatalf("test-vet job is not a map")
	}

	steps, ok := testVetJob["steps"].([]interface{})
	if !ok {
		t.Fatalf("test-vet job 'steps' is not a list")
	}

	for i, step := range steps {
		stepMap, ok := step.(map[string]interface{})
		if !ok {
			continue
		}

		uses, exists := stepMap["uses"]
		if !exists {
			continue
		}

		usesStr, ok := uses.(string)
		if !ok {
			continue
		}

		// Check for deprecated or very old versions
		if strings.Contains(usesStr, "@v1") {
			t.Errorf("Step %d uses old action version (v1): %s", i, usesStr)
		}
		if strings.Contains(usesStr, "@v2") {
			t.Errorf("Step %d uses old action version (v2): %s", i, usesStr)
		}

		// Verify it doesn't use SHA directly (which is less maintainable)
		parts := strings.Split(usesStr, "@")
		if len(parts) == 2 && len(parts[1]) == 40 {
			// Likely a SHA
			t.Errorf("Step %d pins action to SHA instead of version tag: %s", i, usesStr)
		}
	}
}

// TestCIWorkflowBranchesConsistency verifies push and PR target same branches.
func TestCIWorkflowBranchesConsistency(t *testing.T) {
	ciPath := filepath.Join("..", "..", ".github", "workflows", "ci.yml")
	data, err := os.ReadFile(ciPath)
	if err != nil {
		t.Fatalf("failed to read CI workflow: %v", err)
	}

	var workflow map[string]interface{}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("failed to parse CI workflow: %v", err)
	}

	on, ok := workflow["on"].(map[string]interface{})
	if !ok {
		t.Fatalf("'on' field is not a map")
	}

	// Extract push branches
	var pushBranches []string
	if push, exists := on["push"]; exists {
		if pushMap, ok := push.(map[string]interface{}); ok {
			if branches, exists := pushMap["branches"]; exists {
				if branchList, ok := branches.([]interface{}); ok {
					for _, b := range branchList {
						if bStr, ok := b.(string); ok {
							pushBranches = append(pushBranches, bStr)
						}
					}
				}
			}
		}
	}

	// Extract PR branches
	var prBranches []string
	if pr, exists := on["pull_request"]; exists {
		if prMap, ok := pr.(map[string]interface{}); ok {
			if branches, exists := prMap["branches"]; exists {
				if branchList, ok := branches.([]interface{}); ok {
					for _, b := range branchList {
						if bStr, ok := b.(string); ok {
							prBranches = append(prBranches, bStr)
						}
					}
				}
			}
		}
	}

	// Verify they're the same
	if len(pushBranches) != len(prBranches) {
		t.Errorf("push and pull_request should target same branches, got push=%v pr=%v", pushBranches, prBranches)
	}

	for i := range pushBranches {
		if i >= len(prBranches) || pushBranches[i] != prBranches[i] {
			t.Errorf("branch mismatch at index %d: push=%v pr=%v", i, pushBranches, prBranches)
		}
	}
}

// TestCIWorkflowStepNamesAreDescriptive verifies step names are meaningful.
func TestCIWorkflowStepNamesAreDescriptive(t *testing.T) {
	ciPath := filepath.Join("..", "..", ".github", "workflows", "ci.yml")
	data, err := os.ReadFile(ciPath)
	if err != nil {
		t.Fatalf("failed to read CI workflow: %v", err)
	}

	var workflow map[string]interface{}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("failed to parse CI workflow: %v", err)
	}

	jobs, ok := workflow["jobs"].(map[string]interface{})
	if !ok {
		t.Fatalf("jobs field is not a map")
	}

	testVet, exists := jobs["test-vet"]
	if !exists {
		t.Fatalf("test-vet job not found")
	}

	testVetJob, ok := testVet.(map[string]interface{})
	if !ok {
		t.Fatalf("test-vet job is not a map")
	}

	steps, ok := testVetJob["steps"].([]interface{})
	if !ok {
		t.Fatalf("test-vet job 'steps' is not a list")
	}

	for i, step := range steps {
		stepMap, ok := step.(map[string]interface{})
		if !ok {
			continue
		}

		name, exists := stepMap["name"]
		if !exists {
			t.Errorf("Step %d is missing a name field", i)
			continue
		}

		nameStr, ok := name.(string)
		if !ok {
			t.Errorf("Step %d name is not a string: %v", i, name)
			continue
		}

		// Verify name is not empty or too vague
		if strings.TrimSpace(nameStr) == "" {
			t.Errorf("Step %d has empty name", i)
		}
		if len(nameStr) < 3 {
			t.Errorf("Step %d has too short name: %q", i, nameStr)
		}

		// Check for vague names
		vaguePrefixes := []string{"Step", "Run step", "Execute"}
		for _, vague := range vaguePrefixes {
			if strings.HasPrefix(nameStr, vague) && len(nameStr) < 15 {
				t.Errorf("Step %d has vague name: %q", i, nameStr)
			}
		}
	}
}

// TestCIWorkflowNoDeprecatedSyntax verifies no deprecated workflow syntax is used.
func TestCIWorkflowNoDeprecatedSyntax(t *testing.T) {
	ciPath := filepath.Join("..", "..", ".github", "workflows", "ci.yml")
	data, err := os.ReadFile(ciPath)
	if err != nil {
		t.Fatalf("failed to read CI workflow: %v", err)
	}

	content := string(data)

	// Check for deprecated syntax patterns
	deprecatedPatterns := []string{
		"::set-output",      // Deprecated in favor of GITHUB_OUTPUT
		"::save-state",      // Deprecated in favor of GITHUB_STATE
		"::add-path",        // Deprecated in favor of GITHUB_PATH
		"actions/setup-node@v1", // Very old version
	}

	for _, pattern := range deprecatedPatterns {
		if strings.Contains(content, pattern) {
			t.Errorf("CI workflow uses deprecated syntax: %s", pattern)
		}
	}
}

// TestCIWorkflowConcurrencyGroupFormat verifies concurrency group is properly formatted.
func TestCIWorkflowConcurrencyGroupFormat(t *testing.T) {
	ciPath := filepath.Join("..", "..", ".github", "workflows", "ci.yml")
	data, err := os.ReadFile(ciPath)
	if err != nil {
		t.Fatalf("failed to read CI workflow: %v", err)
	}

	var workflow map[string]interface{}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("failed to parse CI workflow: %v", err)
	}

	concurrency, exists := workflow["concurrency"]
	if !exists {
		t.Skip("No concurrency configuration")
	}

	concurrencyMap, ok := concurrency.(map[string]interface{})
	if !ok {
		t.Fatalf("concurrency should be a map")
	}

	group, exists := concurrencyMap["group"]
	if !exists {
		t.Error("concurrency group is required")
		return
	}

	groupStr, ok := group.(string)
	if !ok {
		t.Error("concurrency group should be a string")
		return
	}

	// Verify group uses GitHub context variables
	if !strings.Contains(groupStr, "${{") {
		t.Error("concurrency group should use GitHub context variables (e.g., ${{ github.ref }})")
	}

	// Verify it includes workflow or ref context for proper isolation
	hasContext := strings.Contains(groupStr, "github.workflow") ||
		strings.Contains(groupStr, "github.ref") ||
		strings.Contains(groupStr, "github.head_ref")

	if !hasContext {
		t.Error("concurrency group should include github.workflow or github.ref for proper isolation")
	}
}

// TestCIWorkflowJobNamesFollowConvention verifies job names use kebab-case.
func TestCIWorkflowJobNamesFollowConvention(t *testing.T) {
	ciPath := filepath.Join("..", "..", ".github", "workflows", "ci.yml")
	data, err := os.ReadFile(ciPath)
	if err != nil {
		t.Fatalf("failed to read CI workflow: %v", err)
	}

	var workflow map[string]interface{}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("failed to parse CI workflow: %v", err)
	}

	jobs, ok := workflow["jobs"].(map[string]interface{})
	if !ok {
		t.Fatalf("jobs field is not a map")
	}

	for jobName := range jobs {
		// Check if job name uses kebab-case (lowercase with hyphens)
		if strings.ToLower(jobName) != jobName {
			t.Errorf("Job name %q should be lowercase", jobName)
		}

		if strings.Contains(jobName, "_") {
			t.Errorf("Job name %q should use kebab-case (hyphens), not snake_case (underscores)", jobName)
		}

		if strings.Contains(jobName, " ") {
			t.Errorf("Job name %q should not contain spaces", jobName)
		}
	}
}

// TestCIWorkflowNoEmptySteps verifies no steps are empty or malformed.
func TestCIWorkflowNoEmptySteps(t *testing.T) {
	ciPath := filepath.Join("..", "..", ".github", "workflows", "ci.yml")
	data, err := os.ReadFile(ciPath)
	if err != nil {
		t.Fatalf("failed to read CI workflow: %v", err)
	}

	var workflow map[string]interface{}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("failed to parse CI workflow: %v", err)
	}

	jobs, ok := workflow["jobs"].(map[string]interface{})
	if !ok {
		t.Fatalf("jobs field is not a map")
	}

	for jobName, job := range jobs {
		jobMap, ok := job.(map[string]interface{})
		if !ok {
			continue
		}

		steps, ok := jobMap["steps"].([]interface{})
		if !ok {
			continue
		}

		for i, step := range steps {
			stepMap, ok := step.(map[string]interface{})
			if !ok {
				t.Errorf("Job %s step %d is not a map", jobName, i)
				continue
			}

			// Each step must have either 'uses' or 'run'
			_, hasUses := stepMap["uses"]
			_, hasRun := stepMap["run"]

			if !hasUses && !hasRun {
				t.Errorf("Job %s step %d has neither 'uses' nor 'run' field", jobName, i)
			}

			// If it has 'run', verify it's not empty
			if hasRun {
				runCmd, ok := stepMap["run"].(string)
				if !ok || strings.TrimSpace(runCmd) == "" {
					t.Errorf("Job %s step %d has empty 'run' command", jobName, i)
				}
			}
		}
	}
}