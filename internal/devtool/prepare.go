package devtool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

const cliModule = "github.com/laputacn/fuloot-cli"

type buildLock struct {
	Repository string `json:"repository"`
	Ref        string `json:"ref"`
	Commit     string `json:"commit"`
	CLI        string `json:"cli_version"`
	Toolchain  string `json:"toolchain"`
}

// Prepare a standalone business checkout without needing the workspace registry.
// Resolve dependencies in a temporary modfile; failed downloads leave the
// project's existing go.mod, go.sum and policy intact.
func prepareProject(ctx context.Context, dir, source string, lock buildLock) error {
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(lock.Commit) {
		return errors.New("prepare requires a full Git commit SHA")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	policy, err := os.ReadFile(filepath.Join(source, ".fuloot-toolchain"))
	if err != nil {
		return err
	}
	lock.Toolchain = strings.TrimSpace(string(policy))
	if !versionRE.MatchString(lock.Toolchain) {
		return fmt.Errorf("invalid CLI toolchain %q", lock.Toolchain)
	}
	goBin, err := goExecutable()
	if err != nil {
		return err
	}
	run := func(args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, goBin, args...)
		cmd.Dir = dir
		cmd.Env = environment(map[string]string{"GOTOOLCHAIN": lock.Toolchain, "GOWORK": "off", "GOFLAGS": ""})
		cmd.Stderr = os.Stderr
		output, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("go %s: %w", strings.Join(args, " "), err)
		}
		return output, nil
	}
	mod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".fuloot-sync-*.mod")
	if err != nil {
		return err
	}
	modfile := tmp.Name()
	sumfile := strings.TrimSuffix(modfile, ".mod") + ".sum"
	defer os.Remove(modfile)
	defer os.Remove(sumfile)
	if _, err = tmp.Write(mod); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if sum, err := os.ReadFile(filepath.Join(dir, "go.sum")); err == nil {
		if err := os.WriteFile(sumfile, sum, 0600); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	data, err := run("mod", "edit", "-modfile="+modfile, "-json")
	if err != nil {
		return err
	}
	var module struct {
		Module  struct{ Path string }
		Replace []struct{ Old struct{ Path string } }
	}
	if err := json.Unmarshal(data, &module); err != nil {
		return err
	}
	if module.Module.Path == cliModule {
		return errors.New("prepare is for business modules; build CLI changes locally")
	}
	for _, r := range module.Replace {
		if r.Old.Path == cliModule {
			return errors.New("remove the fuloot-cli replace directive before using online CLI builds")
		}
	}
	if _, err := run("mod", "edit", "-modfile="+modfile, "-go="+strings.TrimPrefix(lock.Toolchain, "go"), "-toolchain=none"); err != nil {
		return err
	}
	if _, err := run("get", "-modfile="+modfile, cliModule+"@"+lock.Commit); err != nil {
		return err
	}
	data, err = run("list", "-mod=readonly", "-modfile="+modfile, "-m", "-json", cliModule)
	if err != nil {
		return err
	}
	var resolved struct {
		Path, Version string
		Replace       *json.RawMessage
	}
	if err := json.Unmarshal(data, &resolved); err != nil {
		return err
	}
	if resolved.Path != cliModule || resolved.Version == "" || resolved.Replace != nil {
		return errors.New("online CLI did not resolve to a versioned module")
	}
	lock.CLI = resolved.Version
	// go get must not silently select another CLI version due to graph conflicts.
	data, err = run("list", "-mod=readonly", "-modfile="+modfile, "-m", "-f", "{{.Version}}", cliModule+"@"+lock.Commit)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(data)) != lock.CLI {
		return errors.New("selected CLI module differs from the fetched Git commit")
	}
	updatedMod, err := os.ReadFile(modfile)
	if err != nil {
		return err
	}
	updatedMod = toolchainLine.ReplaceAll(updatedMod, nil)
	if string(goLine.Find(updatedMod)) != "go "+strings.TrimPrefix(lock.Toolchain, "go") {
		return errors.New("module dependencies require a different Go version from the CLI policy")
	}
	lockJSON, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	changes := []change{
		{filepath.Join(dir, "go.mod"), updatedMod},
		{filepath.Join(dir, ".fuloot-toolchain"), []byte(lock.Toolchain + "\n")},
		{filepath.Join(dir, ".fuloot-build.json"), append(lockJSON, '\n')},
	}
	if sum, err := os.ReadFile(sumfile); err == nil {
		changes = append(changes, change{filepath.Join(dir, "go.sum"), sum})
	} else if !os.IsNotExist(err) {
		return err
	}
	// Keep existing Docker builds consistent with the synchronized checkout.
	// Direct docker build does not fetch CLI changes; use build.sh to sync first.
	dockerPath := filepath.Join(dir, "Dockerfile")
	if data, err := os.ReadFile(dockerPath); err == nil {
		if !dockerFrom.Match(data) {
			return fmt.Errorf("%s: unrecognized Go builder", dockerPath)
		}
		changes = append(changes, change{dockerPath, dockerFrom.ReplaceAll(data, []byte("FROM golang:"+strings.TrimPrefix(lock.Toolchain, "go")+"-alpine AS builder"))})
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := applyChanges(changes); err != nil {
		return err
	}
	fmt.Printf("Synchronized %s: %s, CLI %s (%s)\n", dir, lock.Toolchain, lock.CLI, lock.Commit)
	return nil
}

func prepareCommand() *cobra.Command {
	var dir, source string
	var lock buildLock
	cmd := &cobra.Command{Use: "prepare", Short: "Sync one project to a fetched online CLI revision", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return prepareProject(cmd.Context(), dir, source, lock)
	}}
	cmd.Flags().StringVar(&dir, "dir", "", "Business module directory")
	cmd.Flags().StringVar(&source, "source", "", "Fetched CLI source directory")
	cmd.Flags().StringVar(&lock.Commit, "commit", "", "Exact fetched CLI commit")
	cmd.Flags().StringVar(&lock.Repository, "repository", "", "CLI repository URL")
	cmd.Flags().StringVar(&lock.Ref, "ref", "", "Requested Git ref")
	for _, name := range []string{"dir", "source", "commit", "repository", "ref"} {
		_ = cmd.MarkFlagRequired(name)
	}
	return cmd
}
