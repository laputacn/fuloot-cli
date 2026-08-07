package fulootcli

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestPrintBannerIncludesProjectInformation(t *testing.T) {
	previousApp := app
	app = App{ProjectName: "fuloot-plugin-server", PluginMode: true, Version: "2.0.0"}
	t.Cleanup(func() { app = previousApp })

	output := captureStdout(t, func() {
		printBanner("start", "8091", "release")
	})

	for _, expected := range []string{
		"project name", "fuloot-plugin-server",
		"plugin mode", "enabled",
		"fuloot version", "2.0.0",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("banner output does not contain %q:\n%s", expected, output)
		}
	}
}

func TestPrintBannerUsesBackwardCompatibleDefaults(t *testing.T) {
	previousApp := app
	app = App{}
	t.Cleanup(func() { app = previousApp })

	output := captureStdout(t, func() {
		printBanner("status", "8090", "debug")
	})

	for _, expected := range []string{"FulootAPI", "disabled"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("banner output does not contain default %q:\n%s", expected, output)
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
