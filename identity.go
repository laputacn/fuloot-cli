package fulootcli

import "strings"

// System releases are maintained here so business builds follow the online CLI.
// A project selects its generation explicitly, independently of plugin enablement.
const (
	FulootV1Version = "1.0.0"
	FulootV2Version = "2.0.0"
)

func appVersion(a App) string {
	return defaultString(strings.TrimSpace(a.AppVersion), defaultString(strings.TrimSpace(a.Version), "dev"))
}

func printIdentity(a App) {
	project := defaultString(strings.TrimSpace(a.ProjectName), "FulootAPI")
	printRow("project name", project)
	printRow("app name", defaultString(strings.TrimSpace(a.AppName), project))
	if strings.TrimSpace(a.PluginName) != "" || strings.TrimSpace(a.PluginVersion) != "" {
		printRow("plugin name", defaultString(strings.TrimSpace(a.PluginName), "-"))
		printRow("plugin version", defaultString(strings.TrimSpace(a.PluginVersion), "-"))
	}
}

func printVersions(a App) {
	printRow("fuloot version", defaultString(strings.TrimSpace(a.FulootVersion), FulootV1Version))
	printRow("app version", appVersion(a))
}
