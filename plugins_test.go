package fulootcli

import (
	"strings"
	"testing"
)

func TestPrintPluginsIncludesModuleAndRows(t *testing.T) {
	output := captureStdout(t, func() {
		PrintPlugins(PluginModuleInfo{Name: "Fuloot Plugins", Enabled: true, RuntimeProtocol: "v1", InstallDir: "/opt/fuloot/plugins", RuntimeDir: "/opt/fuloot/runtime/plugins"}, []PluginInfo{
			{Name: "农场游戏", ID: "farm", Version: "0.1.0", Status: "running", PID: 28431, Healthy: true},
			{Name: "会员卡", ID: "member-card", Version: "1.0.0", Status: "stopped"},
		})
	})
	for _, expected := range []string{"Fuloot", "Plugins", "module name", "runtime protocol", "插件名称", "农场游戏", "farm", "28431", "运行中"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("plugins output does not contain %q:\n%s", expected, output)
		}
	}
}

func TestPluginsCommandRegistration(t *testing.T) {
	previousApp := app
	app = App{StartFunc: func(string) {}, PluginsFunc: func() (PluginModuleInfo, []PluginInfo, error) { return PluginModuleInfo{}, nil, nil }}
	t.Cleanup(func() { app = previousApp })
	command, _, err := buildRootCmd().Find([]string{"plugins"})
	if err != nil || command == nil || command.Name() != "plugins" {
		t.Fatalf("plugins command not registered: command=%v err=%v", command, err)
	}
}
