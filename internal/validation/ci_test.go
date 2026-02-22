package validation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestCIWorkflowExists verifies the CI workflow file exists at the expected path.
func TestCIWorkflowExists(t *testing.T) {
	ciPath := filepath.Join("..", "..", ".github", "workflows", "ci.yml")
	if _, err := os.Stat(ciPath); err != nil {
		t.Fatalf("CI workflow file does not exist at %s: %v", ciPath, err)
	}
}

// TestCIWorkflowValidYAML verifies the CI workflow is valid YAML.
func TestCIWorkflowValidYAML(t *testing.T) {
	ciPath := filepath.Join("..", "..", ".github", "workflows", "ci.yml")
	data, err := os.ReadFile(ciPath)
	if err != nil {
		t.Fatalf("failed to read CI workflow: %v", err)
	}

	var workflow map[string]interface{}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("CI workflow is not valid YAML: %v", err)
	}
}

// TestCIWorkflowStructure verifies the CI workflow has the expected structure.
func TestCIWorkflowStructure(t *testing.T) {
	ciPath := filepath.Join("..", "..", ".github", "workflows", "ci.yml")
	data, err := os.ReadFile(ciPath)
	if err != nil {
		t.Fatalf("failed to read CI workflow: %v", err)
	}

	var workflow map[string]interface{}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("failed to parse CI workflow: %v", err)
	}

	// Verify required top-level fields
	requiredFields := []string{"name", "on", "jobs"}
	for _, field := range requiredFields {
		if _, exists := workflow[field]; !exists {
			t.Errorf("CI workflow missing required field: %s", field)
		}
	}

	// Verify workflow name
	if name, ok := workflow["name"].(string); !ok || name != "CI" {
		t.Errorf("expected workflow name 'CI', got %v", workflow["name"])
	}

	// Verify jobs exist
	jobs, ok := workflow["jobs"].(map[string]interface{})
	if !ok {
		t.Fatalf("jobs field is not a map")
	}

	if len(jobs) == 0 {
		t.Error("CI workflow has no jobs defined")
	}
}

// TestCIWorkflowTriggers verifies the CI workflow triggers on the correct events.
func TestCIWorkflowTriggers(t *testing.T) {
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

	// Verify push trigger
	if push, exists := on["push"]; !exists {
		t.Error("CI workflow missing 'push' trigger")
	} else if pushMap, ok := push.(map[string]interface{}); ok {
		if branches, exists := pushMap["branches"]; exists {
			branchList, ok := branches.([]interface{})
			if !ok || len(branchList) == 0 {
				t.Error("push trigger should have branches defined")
			}
		}
	}

	// Verify pull_request trigger
	if pr, exists := on["pull_request"]; !exists {
		t.Error("CI workflow missing 'pull_request' trigger")
	} else if prMap, ok := pr.(map[string]interface{}); ok {
		if branches, exists := prMap["branches"]; exists {
			branchList, ok := branches.([]interface{})
			if !ok || len(branchList) == 0 {
				t.Error("pull_request trigger should have branches defined")
			}
		}
	}
}

// TestCIWorkflowTestVetJob verifies the test-vet job configuration.
func TestCIWorkflowTestVetJob(t *testing.T) {
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

	// Verify test-vet job exists
	testVet, exists := jobs["test-vet"]
	if !exists {
		t.Fatalf("CI workflow missing 'test-vet' job")
	}

	testVetJob, ok := testVet.(map[string]interface{})
	if !ok {
		t.Fatalf("test-vet job is not a map")
	}

	// Verify runs-on
	if runsOn, exists := testVetJob["runs-on"]; !exists {
		t.Error("test-vet job missing 'runs-on' field")
	} else if runsOnStr, ok := runsOn.(string); !ok || runsOnStr == "" {
		t.Errorf("test-vet job 'runs-on' should be a non-empty string, got %v", runsOn)
	}

	// Verify timeout-minutes
	if timeout, exists := testVetJob["timeout-minutes"]; !exists {
		t.Error("test-vet job missing 'timeout-minutes' field")
	} else if _, ok := timeout.(int); !ok {
		t.Errorf("test-vet job 'timeout-minutes' should be an integer, got %v", timeout)
	}

	// Verify steps
	steps, ok := testVetJob["steps"].([]interface{})
	if !ok {
		t.Fatalf("test-vet job 'steps' is not a list")
	}

	if len(steps) == 0 {
		t.Error("test-vet job has no steps defined")
	}
}

// TestCIWorkflowRequiredSteps verifies all required steps are present.
func TestCIWorkflowRequiredSteps(t *testing.T) {
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

	// Build a map of step names and commands
	stepNames := make([]string, 0, len(steps))
	runCommands := make([]string, 0)

	for _, step := range steps {
		stepMap, ok := step.(map[string]interface{})
		if !ok {
			continue
		}

		if name, exists := stepMap["name"]; exists {
			if nameStr, ok := name.(string); ok {
				stepNames = append(stepNames, nameStr)
			}
		}

		if run, exists := stepMap["run"]; exists {
			if runStr, ok := run.(string); ok {
				runCommands = append(runCommands, runStr)
			}
		}
	}

	// Verify required steps
	requiredSteps := map[string]bool{
		"Checkout":  false,
		"Setup Go":  false,
		"Run tests": false,
		"Run vet":   false,
	}

	for _, name := range stepNames {
		for reqStep := range requiredSteps {
			if name == reqStep {
				requiredSteps[reqStep] = true
			}
		}
	}

	for step, found := range requiredSteps {
		if !found {
			t.Errorf("Required step '%s' not found in CI workflow", step)
		}
	}

	// Verify test and vet commands
	hasGoTest := false
	hasGoVet := false

	for _, cmd := range runCommands {
		if strings.Contains(cmd, "go test") {
			hasGoTest = true
		}
		if strings.Contains(cmd, "go vet") {
			hasGoVet = true
		}
	}

	if !hasGoTest {
		t.Error("CI workflow missing 'go test' command")
	}
	if !hasGoVet {
		t.Error("CI workflow missing 'go vet' command")
	}
}

// TestCIWorkflowUsesStandardActions verifies standard GitHub Actions are used.
func TestCIWorkflowUsesStandardActions(t *testing.T) {
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

	usedActions := make([]string, 0)
	for _, step := range steps {
		stepMap, ok := step.(map[string]interface{})
		if !ok {
			continue
		}

		if uses, exists := stepMap["uses"]; exists {
			if usesStr, ok := uses.(string); ok {
				usedActions = append(usedActions, usesStr)
			}
		}
	}

	// Verify checkout action
	hasCheckout := false
	for _, action := range usedActions {
		if strings.HasPrefix(action, "actions/checkout@") {
			hasCheckout = true
			break
		}
	}
	if !hasCheckout {
		t.Error("CI workflow should use actions/checkout action")
	}

	// Verify setup-go action
	hasSetupGo := false
	for _, action := range usedActions {
		if strings.HasPrefix(action, "actions/setup-go@") {
			hasSetupGo = true
			break
		}
	}
	if !hasSetupGo {
		t.Error("CI workflow should use actions/setup-go action")
	}
}

// TestCIWorkflowGoVersionConfiguration verifies Go version setup.
func TestCIWorkflowGoVersionConfiguration(t *testing.T) {
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

	// Find setup-go step and verify configuration
	foundSetupGo := false
	for _, step := range steps {
		stepMap, ok := step.(map[string]interface{})
		if !ok {
			continue
		}

		uses, usesExists := stepMap["uses"]
		if !usesExists {
			continue
		}

		usesStr, ok := uses.(string)
		if !ok || !strings.HasPrefix(usesStr, "actions/setup-go@") {
			continue
		}

		foundSetupGo = true

		// Verify 'with' configuration
		with, withExists := stepMap["with"]
		if !withExists {
			t.Error("setup-go step missing 'with' configuration")
			continue
		}

		withMap, ok := with.(map[string]interface{})
		if !ok {
			t.Error("setup-go 'with' should be a map")
			continue
		}

		// Should use go-version-file
		if goVersionFile, exists := withMap["go-version-file"]; exists {
			if gvf, ok := goVersionFile.(string); ok && gvf == "go.mod" {
				// Good: using go.mod for version
			} else {
				t.Errorf("go-version-file should be 'go.mod', got %v", goVersionFile)
			}
		}

		// Verify cache is enabled
		if cache, exists := withMap["cache"]; exists {
			if cacheVal, ok := cache.(bool); ok && cacheVal {
				// Good: cache is enabled
			} else {
				t.Error("setup-go cache should be enabled (true)")
			}
		}
	}

	if !foundSetupGo {
		t.Error("setup-go step not found in workflow")
	}
}

// TestCIWorkflowConcurrencyConfiguration verifies concurrency settings.
func TestCIWorkflowConcurrencyConfiguration(t *testing.T) {
	ciPath := filepath.Join("..", "..", ".github", "workflows", "ci.yml")
	data, err := os.ReadFile(ciPath)
	if err != nil {
		t.Fatalf("failed to read CI workflow: %v", err)
	}

	var workflow map[string]interface{}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("failed to parse CI workflow: %v", err)
	}

	// Verify concurrency configuration exists
	concurrency, exists := workflow["concurrency"]
	if !exists {
		t.Error("CI workflow missing 'concurrency' configuration")
		return
	}

	concurrencyMap, ok := concurrency.(map[string]interface{})
	if !ok {
		t.Error("concurrency should be a map")
		return
	}

	// Verify group is defined
	if group, exists := concurrencyMap["group"]; !exists {
		t.Error("concurrency missing 'group' field")
	} else if groupStr, ok := group.(string); !ok || groupStr == "" {
		t.Error("concurrency group should be a non-empty string")
	}

	// Verify cancel-in-progress
	if cancelInProgress, exists := concurrencyMap["cancel-in-progress"]; exists {
		if _, ok := cancelInProgress.(bool); !ok {
			t.Error("cancel-in-progress should be a boolean")
		}
	}
}

// TestCIWorkflowTestCommand verifies the test command runs all packages.
func TestCIWorkflowTestCommand(t *testing.T) {
	ciPath := filepath.Join("..", "..", ".github", "workflows", "ci.yml")
	data, err := os.ReadFile(ciPath)
	if err != nil {
		t.Fatalf("failed to read CI workflow: %v", err)
	}

	content := string(data)

	// Verify go test runs on all packages
	if !strings.Contains(content, "go test ./...") {
		t.Error("CI workflow should run 'go test ./...' to test all packages")
	}

	// Verify go vet runs on all packages
	if !strings.Contains(content, "go vet ./...") {
		t.Error("CI workflow should run 'go vet ./...' to vet all packages")
	}
}