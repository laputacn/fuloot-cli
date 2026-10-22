// Package fulootcli provides a CLI scaffold with built-in branding,
// daemon management (start/stop/restart/status), and version display.
package fulootcli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// App describes the application to be managed by the CLI.
type App struct {
	// ProjectName is displayed in the startup and status banners.
	// When empty, it defaults to "FulootAPI" for backward compatibility.
	ProjectName string

	// PluginMode indicates whether the managed application has plugin mode enabled.
	PluginMode bool

	// Version string, typically injected via -ldflags at build time.
	Version string

	// StartFunc is called to run the server in foreground mode.
	// modeOverride is the value of --mode flag; empty string means not set.
	StartFunc func(modeOverride string)

	// InfoFunc is called before displaying the banner (daemon start / status).
	// It should load configuration and return the listen port and run mode.
	// modeOverride is the value of --mode flag; empty string means not set.
	InfoFunc func(modeOverride string) (port, mode string)

	// PluginsFunc returns plugin-module information and locally installed plugins.
	// When nil, the plugins command is not registered.
	PluginsFunc func() (PluginModuleInfo, []PluginInfo, error)
}

var (
	app        App
	pidFile    string
	daemonMode bool
	runMode    string
)

func buildRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "fuloot",
		Short: "FulootAPI WebSocket + HTTP API 服务",
		Long:  "FulootAPI 基于 Gin 提供 HTTP API 与 WebSocket。\n不带子命令时默认前台启动 Web 服务。",
		Run: func(cmd *cobra.Command, args []string) {
			app.StartFunc(runMode)
		},
	}

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "打印程序版本",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(os.Stdout, app.Version)
		},
	}

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "启动 HTTP 服务（默认前台，-d 后台）",
		Long: `启动 FulootAPI 服务。默认以前台方式运行。
加 -d 可切换为 daemon 后台模式。使用 --mode 可覆盖配置文件中的运行模式。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if daemonMode {
				return runDaemonStart(pidFile)
			}
			app.StartFunc(runMode)
			return nil
		},
	}

	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "停止后台进程（SIGTERM）",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemonStop(pidFile)
		},
	}

	restartCmd := &cobra.Command{
		Use:   "restart",
		Short: "重启后台服务（先 stop 再 start）",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := runDaemonStop(pidFile); err != nil {
				return err
			}
			return runDaemonStart(pidFile)
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "查看后台进程是否在运行",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemonStatus(pidFile)
		},
	}

	defaultPid := filepath.Join("logs", "fuloot.pid")
	rootCmd.PersistentFlags().StringVar(&pidFile, "pid-file", defaultPid, "PID 文件路径")

	startCmd.Flags().BoolVarP(&daemonMode, "daemon", "d", false, "以 daemon 后台方式运行")
	startCmd.Flags().StringVar(&runMode, "mode", "", "运行模式: debug / release（覆盖 config）")

	rootCmd.AddCommand(versionCmd, startCmd, stopCmd, restartCmd, statusCmd)
	rootCmd.AddCommand(&cobra.Command{
		Use:   "build-info",
		Short: "打印当前可执行文件的 Go 编译信息（JSON）",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			info, ok := debug.ReadBuildInfo()
			if !ok {
				return fmt.Errorf("build information unavailable")
			}
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(info)
		},
	})
	if app.PluginsFunc != nil {
		rootCmd.AddCommand(&cobra.Command{
			Use:   "plugins",
			Short: "显示插件模块及已安装插件状态",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				module, plugins, err := app.PluginsFunc()
				if err != nil {
					return err
				}
				PrintPlugins(module, plugins)
				return nil
			},
		})
	}
	return rootCmd
}

// Run sets up the CLI and executes it. Call this from your main() function.
func Run(a App) {
	app = a
	rootCmd := buildRootCmd()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
