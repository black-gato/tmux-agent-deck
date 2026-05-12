//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var suite *Suite

func TestMain(m *testing.M) {
	if _, err := exec.LookPath("tmux"); err != nil {
		fmt.Fprintln(os.Stderr, "skipping e2e tests: tmux is not available on PATH")
		os.Exit(0)
	}

	root, err := moduleRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "find module root: %v\n", err)
		os.Exit(1)
	}
	tempDir, err := os.MkdirTemp("", "tmux-agent-deck-e2e-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create suite temp dir: %v\n", err)
		os.Exit(1)
	}
	binPath := filepath.Join(tempDir, "tmux-agent-deck")
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir = root
	build.Env = os.Environ()
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build tmux-agent-deck: %v\n%s\n", err, out)
		_ = os.RemoveAll(tempDir)
		os.Exit(1)
	}

	suite = &Suite{
		RootDir: root,
		BinPath: binPath,
		TempDir: tempDir,
		Prefix:  "e2e",
	}
	code := m.Run()
	suite.KillSessionsByPrefix(suite.Prefix)
	_ = os.RemoveAll(tempDir)
	os.Exit(code)
}

func moduleRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("runtime caller unavailable")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		next := filepath.Dir(dir)
		if next == dir {
			return "", fmt.Errorf("go.mod not found from %s", file)
		}
		dir = next
	}
}

func TestHarnessBuildFixtureAndCleanup(t *testing.T) {
	env := suite.NewTest(t)
	fx := env.CreateQAFixture(t)

	if out := env.RunCLI(t, "list", "--json"); !strings.Contains(out, fx.Root.Title) {
		t.Fatalf("list --json did not include root session %q:\n%s", fx.Root.Title, out)
	}
	AssertTmuxSessionExists(t, fx.Root.TmuxSession, true)

	env.CleanupNow(t)
	AssertTmuxSessionExists(t, fx.Root.TmuxSession, false)
	AssertTmuxSessionExists(t, fx.Front.TmuxSession, false)
	AssertTmuxSessionExists(t, fx.Back.TmuxSession, false)
}
