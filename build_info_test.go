package fulootcli

import (
	"encoding/json"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

func TestBuildInfoDoesNotChangeVersionContract(t *testing.T) {
	old := app
	app = App{Version: "2.3.4"}
	t.Cleanup(func() { app = old })
	version := captureStdout(t, func() {
		cmd := buildRootCmd()
		cmd.SetArgs([]string{"version"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	if strings.TrimSpace(version) != "2.3.4" {
		t.Fatalf("version contract changed: %q", version)
	}
	output := captureStdout(t, func() {
		cmd := buildRootCmd()
		cmd.SetArgs([]string{"build-info"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	var info debug.BuildInfo
	if err := json.Unmarshal([]byte(output), &info); err != nil {
		t.Fatal(err)
	}
	if info.GoVersion != runtime.Version() {
		t.Fatalf("wrong build version: %s", info.GoVersion)
	}
}
