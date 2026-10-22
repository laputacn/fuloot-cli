package fulootcli

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

const logo = `
  ______      _               _        _    ____ ___ 
 |  ____|   _| | ___   ___  _| |_     / \  |  _ \_ _|
 | |__ | | | | |/ _ \ / _ \|_   _|   / _ \ | |_) | | 
 |  __|| |_| | | (_) | (_) | | |    / ___ \|  __/| | 
 |_|    \__,_|_|\___/ \___/  |_|   /_/   \_\_|  |___|
`

const (
	author = "Leon"
	email  = "leon@laputa.cc"
)

func printBanner(action, port, mode string) {
	if mode == "" {
		mode = "debug"
	}
	ginMode := "debug"
	if mode != "debug" {
		ginMode = "release"
	}

	fmt.Print("\033[36m")
	fmt.Print(logo)
	fmt.Println(strings.Repeat("─", 52))
	printRow("Author", author)
	printRow("Email", email)
	printIdentity(app)
	printRow("plugin mode", pluginModeStatus())
	printRow("action", action)
	printRow("listen address", "0.0.0.0")
	printRow("listen port", port)
	printRow("pid", fmt.Sprintf("%d", os.Getpid()))
	printRow("gin mode", ginMode)
	printRow("go version", runtime.Version())
	printVersions(app)
	printRow("run mode", mode)
	printRow("config dir", "config/")
	printRow("log dir", "logs/")
	printRow("GOOS/GOARCH", runtime.GOOS+"/"+runtime.GOARCH)
	fmt.Println(strings.Repeat("─", 52))
	fmt.Println()
}

func pluginModeStatus() string {
	if app.PluginMode {
		return "enabled"
	}
	return "disabled"
}

func printRow(label, value string) {
	fmt.Printf("  \033[33m%-24s\033[0m %s\n", label, value)
}

// PrintBanner prints the branded banner with application info.
// Exported so the main app can call it in foreground mode.
func PrintBanner(action, port, mode string) {
	printBanner(action, port, mode)
}
