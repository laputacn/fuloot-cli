//go:build windows

package fulootcli

import "fmt"

func runDaemonStart(string) error {
	return fmt.Errorf("Windows 不支持 daemon 模式，请直接使用「fuloot start」前台运行")
}

func runDaemonStop(string) error {
	return fmt.Errorf("fuloot stop: 当前平台不支持，请关闭运行窗口或使用任务管理器")
}

func runDaemonStatus(string) error {
	return fmt.Errorf("fuloot status: 当前平台不支持，请使用任务管理器查看进程")
}
