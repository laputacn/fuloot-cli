package fulootcli

import (
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const pluginsLogo = `
  ______      _               _     _____  _             _           
 |  ____|   _| | ___   ___  | |_  |  __ \| |_   _  __ _(_)_ __  ___ 
 | |__ | | | | |/ _ \ / _ \ | __| | |__) | | | | |/ _` + "`" + ` | | '_ \/ __|
 |  __|| |_| | | (_) | (_) || |_  |  ___/| | |_| | (_| | | | | \__ \
 |_|    \__,_|_|\___/ \___/  \__| |_|    |_|\__,_|\__, |_|_| |_|___/
                                                   |___/              
`

type PluginModuleInfo struct {
	Name            string
	Enabled         bool
	RuntimeProtocol string
	InstallDir      string
	RuntimeDir      string
	Communication   string
	Version         string
}

type PluginInfo struct {
	Name    string
	ID      string
	Version string
	Status  string
	PID     int
	Healthy bool
	Health  string
}

func PrintPlugins(module PluginModuleInfo, plugins []PluginInfo) {
	rows := append([]PluginInfo(nil), plugins...)
	sort.SliceStable(rows, func(left, right int) bool { return rows[left].ID < rows[right].ID })
	running, stopped, unhealthy := pluginCounts(rows)

	fmt.Print("\033[36m")
	fmt.Print(pluginsLogo)
	fmt.Print("\033[0m")
	fmt.Println(strings.Repeat("─", 72))
	identity := App{ProjectName: "FulootPlug", FulootVersion: FulootV2Version,
		AppName: defaultString(module.Name, "Fuloot Plugin Runtime"), AppVersion: module.Version}
	printIdentity(identity)
	printVersions(identity)
	printPluginRow("go version", runtime.Version())
	printPluginRow("module name", defaultString(module.Name, "Fuloot Plugins"))
	printPluginRow("module status", enabledStatus(module.Enabled))
	printPluginRow("runtime protocol", defaultString(module.RuntimeProtocol, "v1"))
	printPluginRow("install dir", defaultString(module.InstallDir, "/opt/fuloot/plugins"))
	printPluginRow("runtime dir", defaultString(module.RuntimeDir, "/opt/fuloot/runtime/plugins"))
	printPluginRow("communication", defaultString(module.Communication, "Unix Socket"))
	printPluginRow("installed", strconv.Itoa(len(rows)))
	printPluginRow("running", strconv.Itoa(running))
	printPluginRow("stopped", strconv.Itoa(stopped))
	printPluginRow("unhealthy", strconv.Itoa(unhealthy))
	if strings.TrimSpace(module.Version) != "" {
		printPluginRow("module version", module.Version)
	}
	printPluginRow("GOOS/GOARCH", runtime.GOOS+"/"+runtime.GOARCH)
	fmt.Println(strings.Repeat("─", 72))
	fmt.Println()

	if len(rows) == 0 {
		fmt.Println("  暂无已安装插件")
		fmt.Println()
		return
	}
	columns := []int{18, 22, 12, 14, 10}
	printPluginColumns([]string{"插件名称", "插件标识", "版本", "运行状态", "PID", "健康状态"}, columns)
	for _, plugin := range rows {
		pid := "-"
		if plugin.PID > 0 {
			pid = strconv.Itoa(plugin.PID)
		}
		printPluginColumns([]string{defaultString(plugin.Name, plugin.ID), plugin.ID, defaultString(plugin.Version, "-"), pluginStatusLabel(plugin.Status), pid, pluginHealthLabel(plugin)}, columns)
	}
	fmt.Println()
}

func pluginCounts(plugins []PluginInfo) (running, stopped, unhealthy int) {
	for _, plugin := range plugins {
		switch strings.ToLower(strings.TrimSpace(plugin.Status)) {
		case "running":
			running++
		case "stopped":
			stopped++
		default:
			unhealthy++
		}
	}
	return
}

func pluginStatusLabel(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running":
		return "运行中"
	case "stopped":
		return "已停止"
	case "unhealthy":
		return "异常"
	case "incomplete":
		return "不完整"
	case "invalid":
		return "无效"
	default:
		return defaultString(status, "未知")
	}
}

func pluginHealthLabel(plugin PluginInfo) string {
	if strings.TrimSpace(plugin.Health) != "" {
		return plugin.Health
	}
	if plugin.Healthy {
		return "正常"
	}
	switch strings.ToLower(strings.TrimSpace(plugin.Status)) {
	case "stopped":
		return "不可用"
	case "running":
		return "正常"
	case "unhealthy":
		return "失败"
	default:
		return "未检查"
	}
}

func printPluginRow(label, value string) {
	fmt.Printf("  \033[33m%-24s\033[0m %s\n", label, value)
}

func printPluginColumns(values []string, widths []int) {
	fmt.Print("  ")
	for index, value := range values {
		if index < len(widths) {
			fmt.Print(padDisplay(value, widths[index]))
			continue
		}
		fmt.Print(value)
	}
	fmt.Println()
}

func padDisplay(value string, width int) string {
	displayWidth := 0
	for _, current := range value {
		displayWidth++
		if unicode.Is(unicode.Han, current) || unicode.Is(unicode.Hangul, current) || unicode.Is(unicode.Hiragana, current) || unicode.Is(unicode.Katakana, current) {
			displayWidth++
		}
	}
	if displayWidth >= width {
		return value + "  "
	}
	return value + strings.Repeat(" ", width-displayWidth)
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func enabledStatus(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}
