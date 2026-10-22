package devtool

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Exercise the real Go module resolver against a local module proxy. No public
// repository writes, global go env changes, or network dependency is needed.
func TestPrepareFollowsCommitAndBuildsActualCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX integration")
	}
	goBin, err := goExecutable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GO_BINARY", goBin)
	t.Setenv("GOMODCACHE", t.TempDir())
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GOPRIVATE", "")
	t.Setenv("GONOPROXY", "none")
	files := map[string][]byte{}
	commits := []string{strings.Repeat("a", 40), strings.Repeat("b", 40)}
	versions := []string{}
	for i, commit := range commits {
		version := "v0.0.0-20261022000000-" + commit[:12]
		versions = append(versions, version)
		mod := "module " + cliModule + "\n\ngo " + strings.TrimPrefix(runtime.Version(), "go") + "\n"
		info := []byte(fmt.Sprintf(`{"Version":%q,"Time":"2026-10-22T00:00:00Z"}`, version))
		var buf bytes.Buffer
		z := zip.NewWriter(&buf)
		for name, contents := range map[string]string{"go.mod": mod, "cli.go": fmt.Sprintf("package cli\nconst Marker = %q\n", fmt.Sprintf("remote-%d", i))} {
			w, _ := z.Create(cliModule + "@" + version + "/" + name)
			_, _ = w.Write([]byte(contents))
		}
		_ = z.Close()
		prefix := "/" + cliModule + "/@v/"
		files[prefix+commit+".info"] = info
		files[prefix+version+".info"] = info
		files[prefix+version+".mod"] = []byte(mod)
		files[prefix+version+".zip"] = buf.Bytes()
		files[prefix+"list"] = append(files[prefix+"list"], []byte(version+"\n")...)
		files["/"+cliModule+"/@latest"] = info
	}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if data, ok := files[r.URL.Path]; ok {
			_, _ = w.Write(data)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer proxy.Close()
	t.Setenv("GOPROXY", proxy.URL)
	dir, source := t.TempDir(), t.TempDir()
	put(t, filepath.Join(dir, "go.mod"), "module example.com/business\n\ngo 1.26.1\n\nreplace example.com/runtime => ../local-runtime\n")
	put(t, filepath.Join(dir, ".fuloot-toolchain"), "go1.26.1\n")
	put(t, filepath.Join(dir, "main.go"), "package main\nimport (\"fmt\"; cli \""+cliModule+"\")\nfunc main() {fmt.Print(cli.Marker)}\n")
	put(t, filepath.Join(dir, "Dockerfile"), "FROM golang:1.26.1-alpine AS builder\n")
	put(t, filepath.Join(source, ".fuloot-toolchain"), runtime.Version()+"\n")
	for i, commit := range commits {
		if err := prepareProject(context.Background(), dir, source, buildLock{Commit: commit, Repository: "fixture", Ref: "main"}); err != nil {
			t.Fatal(err)
		}
		lockData, _ := os.ReadFile(filepath.Join(dir, ".fuloot-build.json"))
		var lock buildLock
		if err := json.Unmarshal(lockData, &lock); err != nil || lock.CLI != versions[i] || lock.Commit != commit || lock.Toolchain != runtime.Version() {
			t.Fatalf("wrong lock: %s (%v)", lockData, err)
		}
		mod, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
		if !strings.Contains(string(mod), "replace example.com/runtime => ../local-runtime") {
			t.Fatalf("lost unrelated local dependency: %s", mod)
		}
		bin := filepath.Join(dir, "app")
		cmd := exec.Command(goBin, "build", "-buildvcs=false", "-mod=readonly", "-o", bin, ".")
		cmd.Dir = dir
		cmd.Env = environment(map[string]string{"GOTOOLCHAIN": runtime.Version(), "GOWORK": "off", "GOFLAGS": "", "GOOS": runtime.GOOS, "GOARCH": runtime.GOARCH})
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build: %s %v", output, err)
		}
		if err := verifyArtifact(bin, "", runtime.Version()); err != nil {
			t.Fatal(err)
		}
		if output, err := exec.Command(bin).CombinedOutput(); err != nil || string(output) != fmt.Sprintf("remote-%d", i) {
			t.Fatalf("CLI update missing from binary: %s %v", output, err)
		}
	}
	before := map[string]string{}
	for _, name := range []string{"go.mod", "go.sum", ".fuloot-toolchain", ".fuloot-build.json", "Dockerfile"} {
		data, _ := os.ReadFile(filepath.Join(dir, name))
		before[name] = string(data)
	}
	if err := prepareProject(context.Background(), dir, source, buildLock{Commit: strings.Repeat("c", 40)}); err == nil {
		t.Fatal("accepted unavailable CLI commit")
	}
	for name, want := range before {
		data, _ := os.ReadFile(filepath.Join(dir, name))
		if string(data) != want {
			t.Errorf("failed sync changed %s", name)
		}
	}
	put(t, filepath.Join(dir, "go.mod"), before["go.mod"]+"\nreplace "+cliModule+" => ../local-cli\n")
	if err := prepareProject(context.Background(), dir, source, buildLock{Commit: commits[0]}); err == nil || !strings.Contains(err.Error(), "replace") {
		t.Fatalf("local CLI replace must be rejected: %v", err)
	}
}

func TestBootstrapFetchesMainAndPinnedCommit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX integration")
	}
	repo, dir := t.TempDir(), t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = environment(map[string]string{"GIT_AUTHOR_NAME": "Fixture", "GIT_AUTHOR_EMAIL": "fixture@example.com", "GIT_COMMITTER_NAME": "Fixture", "GIT_COMMITTER_EMAIL": "fixture@example.com"})
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git: %s %v", output, err)
		}
		return strings.TrimSpace(string(output))
	}
	git("init", "-q", "-b", "main")
	put(t, filepath.Join(repo, ".fuloot-toolchain"), runtime.Version()+"\n")
	// Test the stable shell contract independently of the central Go resolver.
	put(t, filepath.Join(repo, "scripts", "fuloot-build.sh"), `fuloot_remote_setup() {
  fuloot_use_go "$(cat "$2/.fuloot-toolchain")" || return 1
  cat "$2/marker" > "$1/result"
  printf '%s\n' "$FULOOT_CLI_COMMIT" > "$1/commit"
}
`)
	put(t, filepath.Join(repo, "marker"), "first")
	git("add", ".")
	git("commit", "-qm", "first")
	first := git("rev-parse", "HEAD")
	put(t, filepath.Join(dir, "go.mod"), "module example.com/business\n\ngo 1.26.1\n")
	put(t, filepath.Join(dir, "bootstrap.sh"), string(shellHelper))
	run := func(repository, ref string, wantError bool) {
		t.Helper()
		cmd := exec.Command("sh", "-ec", `. ./bootstrap.sh; fuloot_go_setup "$PWD"`)
		cmd.Dir = dir
		cmd.Env = environment(map[string]string{"FULOOT_CLI_REPOSITORY": repository, "FULOOT_CLI_REF": ref, "FULOOT_CLI_SYNC": "1", "GOTOOLCHAIN": "go1.26.1", "GOOS": "windows", "GOARCH": "arm64"})
		output, err := cmd.CombinedOutput()
		if (err != nil) != wantError {
			t.Fatalf("bootstrap: %s %v", output, err)
		}
	}
	run(repo, "main", false)
	put(t, filepath.Join(repo, "marker"), "second")
	git("add", ".")
	git("commit", "-qm", "second")
	run(repo, "main", false)
	if data, _ := os.ReadFile(filepath.Join(dir, "result")); string(data) != "second" {
		t.Fatalf("did not follow remote main: %s", data)
	}
	run(repo, first, false)
	if data, _ := os.ReadFile(filepath.Join(dir, "result")); string(data) != "first" {
		t.Fatalf("did not honor pinned revision: %s", data)
	}
	run(filepath.Join(repo, "missing"), "main", true)
	_ = os.Remove(filepath.Join(repo, "scripts", "fuloot-build.sh"))
	git("add", ".")
	git("commit", "-qm", "missing bootstrap")
	run(repo, "main", true)
}

func TestSetCLIPolicyWithoutBusinessCheckouts(t *testing.T) {
	c, p := fixture(t)
	p.Name = "cli"
	c.Projects = []Project{p, {Name: "missing", Path: "not-checked-out"}}
	data, _ := json.Marshal(c)
	put(t, c.file, string(data))
	cmd := NewCommand()
	cmd.SetArgs([]string{"--config", c.file, "toolchain", "set", runtime.Version()})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}
