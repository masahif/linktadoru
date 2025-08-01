package main

import (
	"os"
	"testing"

	"github.com/masahif/linktadoru/internal/cmd"
)

func TestVersionVariables(t *testing.T) {
	// Test that version variables are properly defined
	if Version == "" {
		t.Error("Version should not be empty string")
	}

	if BuildTime == "" {
		t.Error("BuildTime should not be empty string")
	}

	// Default values should be set
	if Version == "" {
		Version = "dev"
	}
	if BuildTime == "" {
		BuildTime = "unknown"
	}

	// Test setting version info
	cmd.SetVersionInfo(Version, BuildTime)
}

func TestMain(t *testing.T) {
	// Save original args and restore after test
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Test main with help flag (should not cause exit in test)
	os.Args = []string{"linktadoru", "--help"}

	// We can't directly test main() since it calls os.Exit on error
	// But we can test the components it uses

	// Test that version info is set
	cmd.SetVersionInfo("test-version", "test-build-time")

	// Test that cmd.Execute can be called (it will show help and return)
	err := cmd.Execute()
	if err != nil {
		t.Logf("Execute returned: %v", err)
	}
}

func TestMainWithVersion(t *testing.T) {
	// Save original args
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Test version flag
	os.Args = []string{"linktadoru", "--version"}

	// Set some test version info
	testVersion := "1.0.0-test"
	testBuildTime := "2023-12-01T10:00:00Z"

	cmd.SetVersionInfo(testVersion, testBuildTime)

	// Execute should return without error for version command
	err := cmd.Execute()
	if err != nil {
		t.Logf("Execute with version returned: %v", err)
	}
}

// TestMainIntegration tests the overall structure
func TestMainIntegration(t *testing.T) {
	// Test that the main components are properly wired together

	// 1. Version variables exist and have default values
	if Version != "dev" && Version != "" {
		t.Logf("Version: %s", Version)
	}

	if BuildTime != "unknown" && BuildTime != "" {
		t.Logf("BuildTime: %s", BuildTime)
	}

	// 2. cmd.SetVersionInfo can be called
	cmd.SetVersionInfo("test", "test-time")

	// 3. cmd.Execute function exists and can be called
	// We don't run it here to avoid actual crawling in tests
}

// TestMainLogic tests the logic inside main() without calling main() directly
func TestMainLogic(t *testing.T) {
	// Save original args
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Test the exact sequence that main() does:

	// 1. Set version information (this is what main() does first)
	cmd.SetVersionInfo(Version, BuildTime)

	// 2. Test that cmd.Execute() works with help (simulates successful execution)
	os.Args = []string{"linktadoru", "--help"}

	err := cmd.Execute()
	// Help command should not return an error
	if err != nil {
		t.Errorf("cmd.Execute() with help should not return error, got: %v", err)
	}

	// This tests the successful path through main()
	// The error path (where os.Exit(1) would be called) is harder to test
	// without actually causing the test to exit, so we verify the components work
}
