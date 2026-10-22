package devtool

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

//go:embed fuloot-go.sh
var shellHelper []byte

var versionRE = regexp.MustCompile(`^go1\.[0-9]+\.[0-9]+$`)
var goLine = regexp.MustCompile(`(?m)^go [^\r\n]+`)
var toolchainLine = regexp.MustCompile(`(?m)^toolchain [^\r\n]+\r?\n?`)
var dockerFrom = regexp.MustCompile(`(?m)^FROM golang:[^\s]+ AS builder`)

type Config struct {
	Toolchain string    `json:"toolchain"`
	Projects  []Project `json:"projects"`
	file      string
}

type Project struct {
	Name     string   `json:"name"`
	Path     string   `json:"path"`
	Manifest string   `json:"manifest,omitempty"`
	Targets  []Target `json:"targets"`
}

type Target struct {
	Name     string `json:"name"`
	Script   string `json:"script"`
	Artifact string `json:"artifact"`
	// Binary is the path inside a plugin ZIP, empty for an executable artifact.
	Binary string `json:"binary,omitempty"`
}

func loadConfig(path string) (*Config, error) {
	if path == "" {
		path = os.Getenv("FULOOT_TOOLCHAIN_CONFIG")
	}
	if path == "" {
		dir, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		for {
			candidate := filepath.Join(dir, "fuloot-toolchain.json")
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				return nil, errors.New("provide --config or FULOOT_TOOLCHAIN_CONFIG pointing to fuloot-toolchain.json")
			}
			dir = parent
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	var c Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("config must contain exactly one JSON object")
	}
	if !versionRE.MatchString(c.Toolchain) {
		return nil, fmt.Errorf("invalid toolchain %q; use go1.X.Y", c.Toolchain)
	}
	seen := map[string]bool{}
	for _, p := range c.Projects {
		if !safeRelative(p.Name) || strings.ContainsAny(p.Name, "/\\") || seen[p.Name] || p.Path == "" {
			return nil, fmt.Errorf("invalid/duplicate project %q", p.Name)
		}
		seen[p.Name] = true
		artifacts := map[string]bool{}
		for _, t := range p.Targets {
			if t.Name == "" || !safeRelative(t.Script) || !safeRelative(t.Artifact) || (t.Binary != "" && !safeRelative(t.Binary)) {
				return nil, fmt.Errorf("invalid target in %s", p.Name)
			}
			if artifacts[t.Artifact] {
				return nil, fmt.Errorf("duplicate artifact in %s: %s", p.Name, t.Artifact)
			}
			artifacts[t.Artifact] = true
		}
		if p.Manifest != "" && !safeRelative(p.Manifest) {
			return nil, fmt.Errorf("invalid manifest in %s", p.Name)
		}
	}
	c.file = abs
	return &c, nil
}

func safeRelative(path string) bool {
	return path != "" && filepath.IsLocal(path) && path != "."
}

func (c *Config) dir(p Project) string {
	if filepath.IsAbs(p.Path) {
		return p.Path
	}
	return filepath.Clean(filepath.Join(filepath.Dir(c.file), p.Path))
}

func (c *Config) selectProjects(all bool, names []string) ([]Project, error) {
	if all && len(names) != 0 {
		return nil, errors.New("choose --all or --project, not both")
	}
	if all {
		return c.Projects, nil
	}
	if len(names) == 0 {
		return nil, errors.New("select --all or --project NAME")
	}
	result := []Project{}
	for _, name := range names {
		found := false
		for _, p := range c.Projects {
			if p.Name == name {
				result = append(result, p)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown project %q", name)
		}
	}
	return result, nil
}

type change struct {
	path string
	data []byte
}

func (c *Config) changes(p Project, version string) ([]change, error) {
	dir := c.dir(p)
	mod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return nil, err
	}
	if !goLine.Match(mod) {
		return nil, fmt.Errorf("%s/go.mod has no go directive", dir)
	}
	mod = goLine.ReplaceAll(mod, []byte("go "+strings.TrimPrefix(version, "go")))
	mod = toolchainLine.ReplaceAll(mod, nil)
	changes := []change{
		{filepath.Join(dir, "go.mod"), mod},
		{filepath.Join(dir, ".fuloot-toolchain"), []byte(version + "\n")},
		{filepath.Join(dir, "scripts", "fuloot-go.sh"), shellHelper},
	}
	dockerPath := filepath.Join(dir, "Dockerfile")
	if data, err := os.ReadFile(dockerPath); err == nil {
		if !dockerFrom.Match(data) {
			return nil, fmt.Errorf("%s: unrecognized Go builder; update manually", dockerPath)
		}
		changes = append(changes, change{dockerPath, dockerFrom.ReplaceAll(data, []byte("FROM golang:"+strings.TrimPrefix(version, "go")+"-alpine AS builder"))})
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return changes, nil
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	mode := os.FileMode(0644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".fuloot-write-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// Preflight all projects before writing; restore earlier files on an I/O failure.
func applyChanges(changes []change) error {
	type backup struct {
		path   string
		data   []byte
		exists bool
	}
	backups := []backup{}
	for _, ch := range changes {
		old, err := os.ReadFile(ch.path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		backups = append(backups, backup{ch.path, old, err == nil})
	}
	for i, ch := range changes {
		if err := writeAtomic(ch.path, ch.data); err != nil {
			errs := []error{err}
			for j := i - 1; j >= 0; j-- {
				b := backups[j]
				if b.exists {
					errs = append(errs, writeAtomic(b.path, b.data))
				} else {
					errs = append(errs, os.Remove(b.path))
				}
			}
			return errors.Join(errs...)
		}
	}
	return nil
}

func (c *Config) check(p Project) error {
	changes, err := c.changes(p, c.Toolchain)
	if err != nil {
		return err
	}
	for _, ch := range changes {
		actual, err := os.ReadFile(ch.path)
		if err != nil {
			return err
		}
		if !bytes.Equal(actual, ch.data) {
			return fmt.Errorf("%s differs from policy; run toolchain set %s --all", ch.path, c.Toolchain)
		}
	}
	for _, t := range p.Targets {
		data, err := os.ReadFile(filepath.Join(c.dir(p), t.Script))
		if err != nil {
			return err
		}
		if !bytes.Contains(data, []byte("fuloot_go_setup")) || !bytes.Contains(data, []byte("fuloot_verify_go")) {
			return fmt.Errorf("%s/%s does not enforce the toolchain", p.Name, t.Script)
		}
	}
	return nil
}
