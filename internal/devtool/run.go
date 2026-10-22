package devtool

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func goExecutable() (string, error) {
	if path := os.Getenv("GO_BINARY"); path != "" {
		return exec.LookPath(path)
	}
	if path, err := exec.LookPath("go"); err == nil {
		return path, nil
	}
	if _, err := os.Stat("/usr/local/go/bin/go"); err == nil {
		return "/usr/local/go/bin/go", nil
	}
	return "", errors.New("Go not found; set GO_BINARY")
}

func environment(values map[string]string) []string {
	result := []string{}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, exists := values[key]; !exists {
			result = append(result, entry)
		}
	}
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result
}

func (c *Config) ensureGo(ctx context.Context) (string, error) {
	goBin, err := goExecutable()
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, goBin, "env", "GOVERSION")
	cmd.Dir = filepath.Dir(c.file)
	cmd.Env = environment(map[string]string{"GOTOOLCHAIN": c.Toolchain, "GOWORK": "off"})
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(output)) != c.Toolchain {
		return "", fmt.Errorf("expected %s, got %s", c.Toolchain, output)
	}
	return goBin, nil
}

func execute(ctx context.Context, dir string, env map[string]string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir, cmd.Env = dir, environment(env)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	return cmd.Run()
}

func (c *Config) test(ctx context.Context, p Project, goBin string) error {
	env := map[string]string{"GOTOOLCHAIN": c.Toolchain, "GOWORK": "off", "GOOS": runtime.GOOS, "GOARCH": runtime.GOARCH}
	if err := execute(ctx, c.dir(p), env, goBin, "test", "-mod=readonly", "./..."); err != nil {
		return err
	}
	return execute(ctx, c.dir(p), env, goBin, "vet", "-mod=readonly", "./...")
}

// Inspect executable bytes without executing them (also supports cross builds).
func verifyArtifact(path, binary, version string) error {
	if binary == "" {
		info, err := buildinfo.ReadFile(path)
		if err != nil {
			return err
		}
		if info.GoVersion != version {
			return fmt.Errorf("%s uses %s, expected %s", path, info.GoVersion, version)
		}
		return nil
	}
	r, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()
	found := false
	for _, f := range r.File {
		if f.Name != binary {
			continue
		}
		if found {
			return fmt.Errorf("duplicate executable %s in %s", binary, path)
		}
		found = true
		reader, err := f.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(reader, 512*1024*1024+1))
		reader.Close()
		if err != nil {
			return err
		}
		if len(data) > 512*1024*1024 {
			return errors.New("ZIP executable exceeds 512 MiB")
		}
		info, err := buildinfo.Read(bytes.NewReader(data))
		if err != nil {
			return err
		}
		if info.GoVersion != version {
			return fmt.Errorf("%s:%s uses %s, expected %s", path, binary, info.GoVersion, version)
		}
	}
	if !found {
		return fmt.Errorf("%s missing from %s", binary, path)
	}
	return nil
}

type Artifact struct {
	Project     string `json:"project"`
	Target      string `json:"target"`
	Path        string `json:"path"`
	Version     string `json:"version"`
	Toolchain   string `json:"toolchain"`
	SHA256      string `json:"sha256"`
	TestsPassed bool   `json:"tests_passed"`
	GitCommit   string `json:"git_commit,omitempty"`
	GitDirty    bool   `json:"git_dirty"`
}

func projectVersion(dir string, p Project, override string) (string, error) {
	if override != "" {
		return override, nil
	}
	if p.Manifest == "" {
		return "dev", nil
	}
	data, err := os.ReadFile(filepath.Join(dir, p.Manifest))
	if err != nil {
		return "", err
	}
	var m struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return "", err
	}
	if m.Version == "" {
		return "", errors.New("manifest has no version")
	}
	return m.Version, nil
}

func (c *Config) build(ctx context.Context, p Project, goBin, out, version string, tested bool) ([]Artifact, error) {
	dir := c.dir(p)
	version, err := projectVersion(dir, p, version)
	if err != nil {
		return nil, err
	}
	if strings.ContainsAny(version, " \t\r\n/\\") {
		return nil, errors.New("version must not contain whitespace or path separators")
	}
	results := []Artifact{}
	for _, t := range p.Targets {
		artifact := filepath.Join(out, p.Name, t.Artifact)
		if _, err := os.Stat(artifact); err == nil {
			return results, fmt.Errorf("refusing to overwrite %s; choose a new --output directory", artifact)
		} else if !os.IsNotExist(err) {
			return results, err
		}
		if err := os.MkdirAll(filepath.Dir(artifact), 0755); err != nil {
			return results, err
		}
		// Batch builds consume the checked and tested snapshot. Do not fetch a new
		// CLI between tests and compilation; ordinary project build.sh syncs online.
		env := map[string]string{"GOTOOLCHAIN": c.Toolchain, "GOWORK": "off", "GO_BINARY": goBin, "VERSION": version, "OUTPUT": artifact, "FULOOT_CLI_SYNC": "0"}
		fmt.Printf("BUILD %s/%s -> %s\n", p.Name, t.Name, artifact)
		if err := execute(ctx, dir, env, "sh", t.Script); err != nil {
			return results, err
		}
		if err := verifyArtifact(artifact, t.Binary, c.Toolchain); err != nil {
			return results, err
		}
		f, err := os.Open(artifact)
		if err != nil {
			return results, err
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return results, err
		}
		a := Artifact{Project: p.Name, Target: t.Name, Path: artifact, Version: version, Toolchain: c.Toolchain, SHA256: hex.EncodeToString(h.Sum(nil)), TestsPassed: tested}
		git := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
		git.Dir = dir
		if output, err := git.Output(); err == nil {
			a.GitCommit = strings.TrimSpace(string(output))
		}
		git = exec.CommandContext(ctx, "git", "status", "--porcelain", "--untracked-files=normal", "--", ".")
		git.Dir = dir
		if output, err := git.Output(); err == nil {
			a.GitDirty = len(output) > 0
		}
		results = append(results, a)
	}
	return results, nil
}

func NewCommand() *cobra.Command {
	var configPath string
	root := &cobra.Command{Use: "fuloot-dev", Short: "Manage Fuloot Go toolchains and builds", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().StringVar(&configPath, "config", "", "Path to fuloot-toolchain.json")
	root.AddCommand(prepareCommand())
	toolchain := &cobra.Command{Use: "toolchain", Short: "Check or synchronize build policy"}
	root.AddCommand(toolchain)
	var checkAll bool
	var checkNames []string
	check := &cobra.Command{Use: "check", Args: cobra.NoArgs, Short: "Check checked-in toolchain, module and Docker configuration", RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadConfig(configPath)
		if err != nil {
			return err
		}
		projects, err := c.selectProjects(checkAll, checkNames)
		if err != nil {
			return err
		}
		var errs []error
		for _, p := range projects {
			if err := c.check(p); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", p.Name, err))
				fmt.Printf("FAIL %s: %v\n", p.Name, err)
			} else {
				fmt.Printf("OK   %s %s\n", p.Name, c.Toolchain)
			}
		}
		return errors.Join(errs...)
	}}
	check.Flags().BoolVar(&checkAll, "all", false, "Check every registered project")
	check.Flags().StringSliceVar(&checkNames, "project", nil, "Project names")
	toolchain.AddCommand(check)
	var setAll bool
	set := &cobra.Command{Use: "set VERSION [--all]", Args: cobra.ExactArgs(1), Short: "Update CLI Go policy; optionally synchronize workspace pins", RunE: func(cmd *cobra.Command, args []string) error {
		version := args[0]
		if !strings.HasPrefix(version, "go") {
			version = "go" + version
		}
		if !versionRE.MatchString(version) {
			return errors.New("use a stable Go patch version, e.g. 1.26.10")
		}
		c, err := loadConfig(configPath)
		if err != nil {
			return err
		}
		var changes []change
		projects := c.Projects
		if !setAll {
			projects, err = c.selectProjects(false, []string{"cli"})
			if err != nil {
				return err
			}
		}
		for _, p := range projects {
			cs, err := c.changes(p, version)
			if err != nil {
				return err
			}
			changes = append(changes, cs...)
		}
		c.Toolchain = version
		if _, err := c.ensureGo(cmd.Context()); err != nil {
			return err
		}
		data, err := json.MarshalIndent(c, "", "  ")
		if err != nil {
			return err
		}
		changes = append(changes, change{c.file, append(data, '\n')})
		if err := applyChanges(changes); err != nil {
			return err
		}
		fmt.Printf("Synchronized %d projects to %s. Push the CLI policy, then rebuild and redeploy business services.\n", len(projects), version)
		return nil
	}}
	set.Flags().BoolVar(&setAll, "all", false, "Synchronize all projects")
	toolchain.AddCommand(set)
	verify := &cobra.Command{Use: "verify FILE...", Args: cobra.MinimumNArgs(1), Short: "Read Go build metadata from binaries (without executing)", RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadConfig(configPath)
		if err != nil {
			return err
		}
		binary, _ := cmd.Flags().GetString("binary")
		for _, path := range args {
			if err := verifyArtifact(path, binary, c.Toolchain); err != nil {
				return err
			}
			fmt.Printf("OK %s %s\n", path, c.Toolchain)
		}
		return nil
	}}
	verify.Flags().String("binary", "", "Executable path inside a ZIP archive")
	toolchain.AddCommand(verify)
	for _, mode := range []string{"test", "build"} {
		mode := mode
		var all, runTests bool
		var names []string
		var output, version string
		command := &cobra.Command{Use: mode, Args: cobra.NoArgs, Short: mode + " selected projects using the pinned toolchain", RunE: func(cmd *cobra.Command, args []string) error {
			c, err := loadConfig(configPath)
			if err != nil {
				return err
			}
			projects, err := c.selectProjects(all, names)
			if err != nil {
				return err
			}
			for _, p := range projects {
				if err := c.check(p); err != nil {
					return fmt.Errorf("%s: %w", p.Name, err)
				}
			}
			goBin, err := c.ensureGo(cmd.Context())
			if err != nil {
				return err
			}
			if mode == "build" {
				if output == "" {
					output = filepath.Join(filepath.Dir(c.file), "dist", c.Toolchain+"-"+time.Now().UTC().Format("20060102T150405.000000000Z"))
				}
				output, err = filepath.Abs(output)
				if err != nil {
					return err
				}
				if _, err := os.Stat(filepath.Join(output, "build-report.json")); err == nil {
					return errors.New("output already contains a build report; choose a fresh directory")
				}
			}
			var errs []error
			artifacts := []Artifact{}
			for _, p := range projects {
				if mode == "test" || runTests {
					fmt.Printf("TEST %s\n", p.Name)
					if err := c.test(cmd.Context(), p, goBin); err != nil {
						errs = append(errs, fmt.Errorf("%s tests: %w", p.Name, err))
						continue
					}
				}
				if mode == "build" {
					built, err := c.build(cmd.Context(), p, goBin, output, version, runTests)
					artifacts = append(artifacts, built...)
					if err != nil {
						errs = append(errs, fmt.Errorf("%s build: %w", p.Name, err))
					}
				}
			}
			if mode == "build" {
				failures := []string{}
				for _, err := range errs {
					failures = append(failures, err.Error())
				}
				report := struct {
					Toolchain string     `json:"toolchain"`
					Artifacts []Artifact `json:"artifacts"`
					Errors    []string   `json:"errors"`
				}{c.Toolchain, artifacts, failures}
				data, err := json.MarshalIndent(report, "", "  ")
				if err != nil {
					return err
				}
				if err := writeAtomic(filepath.Join(output, "build-report.json"), append(data, '\n')); err != nil {
					return err
				}
				fmt.Printf("Build report: %s\n", filepath.Join(output, "build-report.json"))
			}
			return errors.Join(errs...)
		}}
		command.Flags().BoolVar(&all, "all", false, "Select every registered project")
		command.Flags().StringSliceVar(&names, "project", nil, "Project names")
		if mode == "build" {
			command.Flags().BoolVar(&runTests, "test", true, "Run Go tests and vet before building")
			command.Flags().StringVar(&output, "output", "", "Fresh output directory")
			command.Flags().StringVar(&version, "version", "", "Application version (default: manifest version, or dev)")
		}
		root.AddCommand(command)
	}
	return root
}
