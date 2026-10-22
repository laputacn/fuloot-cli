package devtool

import (
	"archive/zip"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func put(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) (*Config, Project) {
	t.Helper()
	dir := t.TempDir()
	p := Project{Name: "example", Path: "example", Targets: []Target{{Name: "app", Script: "build.sh", Artifact: "app"}}}
	c := &Config{Toolchain: runtime.Version(), Projects: []Project{p}, file: filepath.Join(dir, "fuloot-toolchain.json")}
	put(t, filepath.Join(c.dir(p), "go.mod"), "module example\n\ngo 1.26.1\n\n// keep this comment\n")
	put(t, filepath.Join(c.dir(p), "main.go"), "package main\nfunc main() {}\n")
	put(t, filepath.Join(c.dir(p), "build.sh"), `#!/bin/sh
set -eu
ROOT="$PWD"
. "$ROOT/scripts/fuloot-go.sh"
fuloot_go_setup "$ROOT"
"$GO_CMD" build -buildvcs=false -o "$OUTPUT" .
fuloot_verify_go "$OUTPUT"
`)
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	put(t, c.file, string(data))
	changes, err := c.changes(p, c.Toolchain)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyChanges(changes); err != nil {
		t.Fatal(err)
	}
	return c, p
}

func TestSetPreflightsAllProjectsBeforeWriting(t *testing.T) {
	c, p := fixture(t)
	before, err := os.ReadFile(filepath.Join(c.dir(p), "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	c.Projects = append(c.Projects, Project{Name: "missing", Path: "missing"})
	data, _ := json.Marshal(c)
	put(t, c.file, string(data))
	cmd := NewCommand()
	cmd.SetArgs([]string{"--config", c.file, "toolchain", "set", "1.26.10", "--all"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected missing project error")
	}
	after, _ := os.ReadFile(filepath.Join(c.dir(p), "go.mod"))
	if string(before) != string(after) {
		t.Fatal("partially changed earlier project")
	}
}

func TestPolicyRejectsDriftAndPreservesDependencies(t *testing.T) {
	c, p := fixture(t)
	mod := filepath.Join(c.dir(p), "go.mod")
	put(t, mod, "module example\n\ngo 1.26.1\ntoolchain go1.27.3\n\nrequire example.com/lib v1.0.0\n// keep me\n")
	changes, err := c.changes(p, c.Toolchain)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyChanges(changes); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(mod)
	if strings.Contains(string(data), "toolchain ") || !strings.Contains(string(data), "require example.com/lib v1.0.0\n// keep me") {
		t.Fatalf("unexpected module change: %s", data)
	}
	if err := c.check(p); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(c.dir(p), ".fuloot-toolchain"), "go1.26.1\n")
	if err := c.check(p); err == nil {
		t.Fatal("accepted stale pin")
	}
}

func TestDockerPinDrift(t *testing.T) {
	c, p := fixture(t)
	put(t, filepath.Join(c.dir(p), "Dockerfile"), "FROM golang:1.26-alpine AS builder\nRUN go build .\n")
	if err := c.check(p); err == nil {
		t.Fatal("accepted floating Docker pin")
	}
	changes, err := c.changes(p, c.Toolchain)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyChanges(changes); err != nil {
		t.Fatal(err)
	}
	if err := c.check(p); err != nil {
		t.Fatal(err)
	}
}

func TestBuildAndInspectBinaryAndZIP(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell build integration")
	}
	c, p := fixture(t)
	goBin, err := goExecutable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GO_BINARY", goBin)
	out := filepath.Join(t.TempDir(), "output")
	cmd := NewCommand()
	cmd.SetArgs([]string{"--config", c.file, "build", "--all", "--output", out})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(out, p.Name, "app")
	if err := verifyArtifact(binary, "", runtime.Version()); err != nil {
		t.Fatal(err)
	}
	if err := verifyArtifact(binary, "", "go1.1.1"); err == nil {
		t.Fatal("accepted mismatched Go binary")
	}
	archive := filepath.Join(t.TempDir(), "plugin.zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	w, err := z.Create("bin/app")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := verifyArtifact(archive, "bin/app", runtime.Version()); err != nil {
		t.Fatal(err)
	}
	if err := verifyArtifact(archive, "bin/app", "go1.1.1"); err == nil {
		t.Fatal("accepted mismatched ZIP binary")
	}
	if err := verifyArtifact(archive, "bin/missing", runtime.Version()); err == nil {
		t.Fatal("accepted ZIP with missing executable")
	}
	cmd = NewCommand()
	cmd.SetArgs([]string{"--config", c.file, "build", "--all", "--output", out})
	if err := cmd.Execute(); err == nil {
		t.Fatal("overwrote existing release output")
	}
}

func TestShellRejectsConflictingToolchain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell")
	}
	c, p := fixture(t)
	cmd := exec.Command("sh", "-c", `. ./scripts/fuloot-go.sh; fuloot_go_setup "$PWD"`)
	cmd.Dir = c.dir(p)
	cmd.Env = environment(map[string]string{"GOTOOLCHAIN": "go1.26.1", "FULOOT_CLI_SYNC": "0"})
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "conflicts") {
		t.Fatalf("expected conflict: %s %v", output, err)
	}
}

func TestSelectionAndPathValidation(t *testing.T) {
	c, _ := fixture(t)
	if _, err := c.selectProjects(true, []string{"example"}); err == nil {
		t.Fatal("ambiguous selection")
	}
	if _, err := c.selectProjects(false, []string{"unknown"}); err == nil {
		t.Fatal("unknown project accepted")
	}
	for _, path := range []string{"../app", "/tmp/app", ""} {
		if safeRelative(path) {
			t.Fatalf("unsafe path accepted: %s", path)
		}
	}
}
