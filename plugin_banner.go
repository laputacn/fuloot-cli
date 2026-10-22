package fulootcli

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// PluginInfoFromManifest reads identity from an embedded manifest. The optional
// build version matches the version written to the packaged manifest by build.sh.
func PluginInfoFromManifest(data []byte, buildVersion string) (PluginInfo, error) {
	var manifest struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return PluginInfo{}, fmt.Errorf("plugin identity: %w", err)
	}
	info := PluginInfo{
		ID: strings.TrimSpace(manifest.ID), Name: strings.TrimSpace(manifest.Name),
		Version: defaultString(strings.TrimSpace(buildVersion), strings.TrimSpace(manifest.Version)),
	}
	if info.ID == "" || info.Name == "" || info.Version == "" {
		return PluginInfo{}, fmt.Errorf("plugin identity requires id, name and version")
	}
	return info, nil
}

// PrintPluginBanner is for standalone gRPC plugin processes. It does not register
// HTTP/daemon commands or mutate the App managed by Run.
func PrintPluginBanner(plugin PluginInfo, endpoint string) {
	a := App{
		ProjectName: "FulootPlug", FulootVersion: FulootV2Version,
		AppName: plugin.ID, AppVersion: plugin.Version,
		PluginName: plugin.Name, PluginVersion: plugin.Version,
	}
	fmt.Print("\033[36m", pluginsLogo, "\033[0m")
	fmt.Println(strings.Repeat("─", 72))
	printRow("Author", author)
	printRow("Email", email)
	printIdentity(a)
	printRow("action", "start")
	printRow("listen endpoint", endpoint)
	printRow("protocol", "gRPC + Protobuf")
	printRow("pid", strconv.Itoa(os.Getpid()))
	printRow("go version", runtime.Version())
	printVersions(a)
	printRow("GOOS/GOARCH", runtime.GOOS+"/"+runtime.GOARCH)
	fmt.Println(strings.Repeat("─", 72))
	fmt.Println()
}
