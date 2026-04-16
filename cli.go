// Package fulootcli provides a CLI scaffold with built-in branding,
// daemon management (start/stop/restart/status), and version display.
package fulootcli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// App describes the application to be managed by the CLI.
type App struct {
	// Version string, typically injected via -ldflags at build time.
	Version string

	// StartFunc is called to run the server in foreground mode.
	// modeOverride is the value of --mode flag; empty string means not set.
	StartFunc func(modeOverride string)

	// InfoFunc is called before displaying the banner (daemon start / status).
	// It should load configuration and return the listen port and run mode.
	// modeOverride is the value of --mode flag; empty string means not set.
	InfoFunc func(modeOverride string) (port, mode string)
}

var (
	app            App
	pidFile        string
	foregroundMode bool
	runMode        string
)

func buildRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "fuloot",
		Short: "FulootAPI WebSocket + HTTP API 服务",
		Long:  "FulootAPI 基于 Gin 提供 HTTP API 与 WebSocket。\n不带子命令时默认启动 Web 服务。",
		Run: func(cmd *cobra.Command, args []string) {
			if err := runDaemonStart(pidFile); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
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
		Short: "启动 HTTP 服务（默认后台，-f 前台）",
		Long: `启动 FulootAPI 服务。默认以后台 daemon 方式运行，仅显示 banner 信息。
加 -f 可切换为前台模式。使用 --mode 可覆盖配置文件中的运行模式。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if foregroundMode {
				app.StartFunc(runMode)
				return nil
			}
			return runDaemonStart(pidFile)
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

	startCmd.Flags().BoolVarP(&foregroundMode, "foreground", "f", false, "以前台方式运行（显示全部日志）")
	startCmd.Flags().StringVar(&runMode, "mode", "", "运行模式: debug / release（覆盖 config）")

	rootCmd.AddCommand(versionCmd, startCmd, stopCmd, restartCmd, statusCmd)
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
