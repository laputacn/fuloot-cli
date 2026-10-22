package fulootcli

import (
	"runtime"
	"strings"
	"testing"
)

func TestPluginManifestIdentity(t *testing.T) {
	data := []byte(`{"id":"fuloot-world","name":"Fuloot World","version":"1.3.15","type":"game"}`)
	for _, override := range []string{"", "1.3.16"} {
		info, err := PluginInfoFromManifest(data, override)
		if err != nil {
			t.Fatal(err)
		}
		want := defaultString(override, "1.3.15")
		if info.ID != "fuloot-world" || info.Name != "Fuloot World" || info.Version != want {
			t.Fatalf("unexpected identity: %+v", info)
		}
		previous := app
		output := captureStdout(t, func() { PrintPluginBanner(info, "/tmp/plugin.sock") })
		if app.AppVersion != previous.AppVersion || app.ProjectName != previous.ProjectName {
			t.Fatal("plugin banner changed the host identity")
		}
		assertBannerRows(t, output, map[string]string{
			"project name": "FulootPlug", "app name": "fuloot-world", "app version": want,
			"plugin name": "Fuloot World", "plugin version": want, "fuloot version": FulootV2Version,
			"listen endpoint": "/tmp/plugin.sock", "go version": runtime.Version(),
		})
		if strings.Contains(output, "gin mode") || strings.Contains(output, "listen port") {
			t.Fatal("gRPC plugin banner claims HTTP server settings")
		}
	}
	for _, invalid := range []string{`{`, `{}`, `{"id":"world","name":"World"}`} {
		if _, err := PluginInfoFromManifest([]byte(invalid), ""); err == nil {
			t.Fatalf("accepted invalid identity %s", invalid)
		}
	}
}

func TestHTTPPluginAndRuntimeIdentity(t *testing.T) {
	previous := app
	app = App{ProjectName: "FulootPlug", FulootVersion: FulootV2Version,
		AppName: "Example App", AppVersion: "3.0.1", PluginName: "Example Plugin", PluginVersion: "4.0.2"}
	t.Cleanup(func() { app = previous })
	output := captureStdout(t, func() { PrintBanner("start", "8091", "release") })
	assertBannerRows(t, output, map[string]string{
		"project name": "FulootPlug", "app name": "Example App", "app version": "3.0.1",
		"plugin name": "Example Plugin", "plugin version": "4.0.2", "fuloot version": FulootV2Version,
	})
	output = captureStdout(t, func() {
		PrintPlugins(PluginModuleInfo{Name: "Fuloot Plugins", Version: "5.0.3"}, nil)
	})
	assertBannerRows(t, output, map[string]string{
		"project name": "FulootPlug", "app name": "Fuloot Plugins", "app version": "5.0.3",
		"fuloot version": FulootV2Version, "module version": "5.0.3", "go version": runtime.Version(),
	})
}
