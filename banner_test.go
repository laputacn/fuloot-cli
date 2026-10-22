package fulootcli

import (
	"io"
	"os"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestBannerSeparatesSystemAndAppVersions(t *testing.T) {
	previous := app
	t.Cleanup(func() { app = previous })
	for _, tc := range []struct {
		name                             string
		value                            App
		system, version, nameValue, mode string
	}{
		{"legacy", App{Version: "1.0.20"}, FulootV1Version, "1.0.20", "FulootAPI", "disabled"},
		{"movie", App{AppName: "RYS Movie", AppVersion: "1.0.20", Version: "old"}, FulootV1Version, "1.0.20", "RYS Movie", "disabled"},
		{"v2-disabled", App{FulootVersion: FulootV2Version, AppName: "Petmini", AppVersion: "3.4.5"}, FulootV2Version, "3.4.5", "Petmini", "disabled"},
		{"v2-enabled", App{FulootVersion: FulootV2Version, PluginMode: true, AppVersion: "3.4.5"}, FulootV2Version, "3.4.5", "FulootAPI", "enabled"},
		{"defaults", App{}, FulootV1Version, "dev", "FulootAPI", "disabled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app = tc.value
			for _, action := range []string{"start", "start -d (daemon)", "status"} {
				output := captureStdout(t, func() { PrintBanner(action, "8086", "produce") })
				assertBannerRows(t, output, map[string]string{
					"project name": "FulootAPI", "app name": tc.nameValue, "fuloot version": tc.system,
					"app version": tc.version, "plugin mode": tc.mode, "action": action, "go version": runtime.Version(),
				})
				if strings.Contains(output, "plugin name") || strings.Contains(output, "plugin version") {
					t.Fatal("host banner incorrectly claims a plugin identity")
				}
			}
			output := captureStdout(t, func() {
				cmd := buildRootCmd()
				cmd.SetArgs([]string{"version"})
				if err := cmd.Execute(); err != nil {
					t.Fatal(err)
				}
			})
			if strings.TrimSpace(output) != tc.version {
				t.Fatalf("version = %q, want %q", output, tc.version)
			}
		})
	}
}

func assertBannerRows(t *testing.T, output string, expected map[string]string) {
	t.Helper()
	clean := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(output, "")
	for label, value := range expected {
		row := regexp.MustCompile(`(?m)^  ` + regexp.QuoteMeta(label) + ` +` + regexp.QuoteMeta(value) + `$`)
		if !row.MatchString(clean) {
			t.Fatalf("missing row %q = %q:\n%s", label, value, clean)
		}
	}
}

func captureStdout(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	previousStdout := os.Stdout
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = previousStdout })

	run()
	if err := writer.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	os.Stdout = previousStdout
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close stdout reader: %v", err)
	}
	return string(output)
}
