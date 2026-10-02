package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// devSignInRoute is the dev sign-in's route pattern, which only a binary that has the dev sign-in holds.
const devSignInRoute = "POST /account/dev-sign-in"

// The release builds the web function as lambda-build.toml says, and its binary must not hold the dev sign-in: a
// binary that did would let anyone sign in as a test user. This builds the web function with the release's own
// command line, and with the rulemartdev tag added, and checks that only the second holds the dev sign-in's route,
// so the check would see it if the release build had it.
func TestTheReleaseBuildOfTheWebFunctionHasNoDevSignIn(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the web function twice")
	}
	tags := releaseBuildTags(t)
	if slices.Contains(tags, "rulemartdev") {
		t.Fatalf("lambda-build.toml builds with the rulemartdev tag: %v", tags)
	}

	release := buildWebFunction(t, tags)
	dev := buildWebFunction(t, append(slices.Clone(tags), "rulemartdev"))

	if bytes.Contains(release, []byte(devSignInRoute)) {
		t.Error("the release build of the web function holds the dev sign-in")
	}
	if !bytes.Contains(dev, []byte(devSignInRoute)) {
		t.Error("a rulemartdev build doesn't hold the dev sign-in's route, so this check can't see it")
	}
}

// releaseBuildTags returns the build tags of the go build command in lambda-build.toml's build script, which builds
// every function.
func releaseBuildTags(t *testing.T) []string {
	t.Helper()
	config, err := os.ReadFile(filepath.Join("..", "..", "lambda-build.toml"))
	if err != nil {
		t.Fatal(err)
	}
	builds := regexp.MustCompile(`(?m)^\s*go build (.*)$`).FindAllSubmatch(config, -1)
	if len(builds) != 1 {
		t.Fatalf("lambda-build.toml has %d go build commands, want 1", len(builds))
	}
	match := regexp.MustCompile(`-tags[ =]([^ ]+)`).FindSubmatch(builds[0][1])
	if match == nil {
		return nil
	}
	return strings.Split(string(match[1]), ",")
}

// buildWebFunction builds cmd/web for Lambda's platform, as the release does, with tags, and returns the binary.
func buildWebFunction(t *testing.T, tags []string) []byte {
	t.Helper()
	out := filepath.Join(t.TempDir(), "bootstrap")
	args := []string{"build", "-trimpath", "-ldflags=-s -w", "-o", out}
	if len(tags) > 0 {
		args = append(args, "-tags", strings.Join(tags, ","))
	}
	cmd := exec.Command("go", append(args, ".")...)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=arm64")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	binary, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return binary
}
