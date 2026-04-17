//go:build !windows

package fulootcli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	daemonLogRelPath = "logs/fuloot-daemon.log"
	stopWaitStep     = 200 * time.Millisecond
	stopWaitMax      = 15 * time.Second
)

func runDaemonStart(pidPath string) error {
	if err := os.MkdirAll(filepath.Dir(pidPath), 0755); err != nil {
		return fmt.Errorf("创建 PID 目录: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(daemonLogRelPath), 0755); err != nil {
		return fmt.Errorf("创建日志目录: %w", err)
	}

	if pid, ok, err := readPIDFile(pidPath); err != nil {
		return err
	} else if ok && processAlive(pid) {
		return fmt.Errorf("已在运行中 (pid=%d)，请先 stop 或使用 --pid-file", pid)
	} else if ok && !processAlive(pid) {
		_ = os.Remove(pidPath)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("解析可执行文件路径: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	logF, err := os.OpenFile(daemonLogRelPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("打开守护日志: %w", err)
	}
	defer logF.Close()

	args := []string{"start"}
	if runMode != "" {
		args = append(args, "--mode", runMode)
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdout = logF
	cmd.Stderr = logF
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动子进程: %w", err)
	}

	pid := cmd.Process.Pid
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", pid)), 0644); err != nil {
		_ = syscall.Kill(pid, syscall.SIGTERM)
		return fmt.Errorf("写入 PID 文件: %w", err)
	}

	port, mode := "8082", ""
	if app.InfoFunc != nil {
		port, mode = app.InfoFunc(runMode)
	}
	printBanner("start -d (daemon)", port, mode)
	fmt.Printf("  \033[32m✓ fuloot 已在后台启动 pid=%d\033[0m\n", pid)
	printRow("daemon log", daemonLogRelPath)
	printRow("pid file", pidPath)
	fmt.Println()
	return nil
}

func runDaemonStop(pidPath string) error {
	pid, ok, err := readPIDFile(pidPath)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(os.Stdout, "未找到 PID 文件，认为未在后台运行")
		return nil
	}
	if !processAlive(pid) {
		_ = os.Remove(pidPath)
		fmt.Fprintf(os.Stdout, "pid=%d 已不存在，已清理 PID 文件\n", pid)
		return nil
	}

	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("发送 SIGTERM 到 pid=%d: %w", pid, err)
	}

	deadline := time.Now().Add(stopWaitMax)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			_ = os.Remove(pidPath)
			fmt.Fprintf(os.Stdout, "已停止 pid=%d\n", pid)
			return nil
		}
		time.Sleep(stopWaitStep)
	}

	return fmt.Errorf("等待 pid=%d 退出超时（仍可能正在关闭，可稍后用 status 查看）", pid)
}

func runDaemonStatus(pidPath string) error {
	pid, ok, err := readPIDFile(pidPath)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Printf("\n  \033[31m✗ 未运行\033[0m（无 PID 文件）\n\n")
		return nil
	}
	if !processAlive(pid) {
		_ = os.Remove(pidPath)
		fmt.Printf("\n  \033[31m✗ 未运行\033[0m（pid=%d 已失效，已清理 PID 文件）\n\n", pid)
		return nil
	}

	port, mode := "8082", ""
	if app.InfoFunc != nil {
		port, mode = app.InfoFunc("")
	}
	printBanner("status", port, mode)
	fmt.Printf("  \033[32m✓ 运行中\033[0m  pid=%-8d  port=%s\n\n", pid, port)
	return nil
}

func readPIDFile(path string) (pid int, ok bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, false, nil
		}
		return 0, false, err
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return 0, false, nil
	}
	pid, err = strconv.Atoi(s)
	if err != nil || pid <= 0 {
		return 0, false, fmt.Errorf("PID 文件内容无效: %q", s)
	}
	return pid, true, nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}
