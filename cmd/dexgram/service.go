package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const serviceTaskName = "Dexgram"

func runServiceCommand(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printServiceHelp(os.Stdout, filepath.Base(os.Args[0]))
		return nil
	}

	switch args[0] {
	case "install":
		return serviceInstall()
	case "uninstall", "remove":
		return serviceUninstall()
	case "start":
		return serviceStart()
	case "stop":
		return serviceStop(false)
	case "restart":
		return serviceRestart()
	case "status":
		return serviceStatus()
	default:
		return fmt.Errorf("unknown service command %q; run %s service --help", args[0], filepath.Base(os.Args[0]))
	}
}

func printServiceHelp(w io.Writer, exe string) {
	fmt.Fprintf(w, `Dexgram Service Commands

  Dexgram service mode is a Windows Task Scheduler user-login task.
  It is not a Windows Service and it runs in the signed-in user's context.

Usage

  %[1]s service install
  %[1]s service start
  %[1]s service stop
  %[1]s service restart
  %[1]s service status
  %[1]s service uninstall

Paths

  Binary: %[2]s
  Config: %[3]s
  Logs:   %[4]s
  State:  %[5]s

Behavior

  install    creates or replaces the scheduled task %[6]q
  start      starts the scheduled task now
  stop       stops the running scheduled task
  restart    stops the task if needed, then starts it again
  status     prints Task Scheduler status and the fixed Dexgram paths
  uninstall  removes the scheduled task

The installer script can place dexgram.exe under %%LOCALAPPDATA%%\Dexgram.
Config, logs, media, and state live under %%APPDATA%%\Dexgram.

`, exe, mustServiceExePath(), mustServiceConfigPath(), mustServiceLogPath(), mustServiceStatePath(), serviceTaskName)
}

func serviceInstall() error {
	exePath := mustServiceExePath()
	configPath := mustServiceConfigPath()
	logPath := mustServiceLogPath()
	if err := os.MkdirAll(filepath.Dir(exePath), 0o755); err != nil {
		return fmt.Errorf("create binary directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	warnings := servicePathWarnings(exePath, configPath)
	taskRun := fmt.Sprintf(`"%s" -config "%s" -log "%s"`, exePath, configPath, logPath)
	out, err := runSchtasks("/Create", "/TN", serviceTaskName, "/SC", "ONLOGON", "/TR", taskRun, "/RL", "LIMITED", "/F")
	if err != nil {
		return err
	}
	printNonEmpty(out)
	for _, warning := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", warning)
	}
	fmt.Printf("installed user-login task %q\n", serviceTaskName)
	fmt.Printf("binary: %s\n", exePath)
	fmt.Printf("config: %s\n", configPath)
	fmt.Printf("log:    %s\n", logPath)
	return nil
}

func serviceUninstall() error {
	out, err := runSchtasks("/Delete", "/TN", serviceTaskName, "/F")
	if err != nil {
		if isTaskNotFound(out) {
			fmt.Printf("scheduled task %q is not installed\n", serviceTaskName)
			return nil
		}
		return err
	}
	printNonEmpty(out)
	fmt.Printf("removed scheduled task %q\n", serviceTaskName)
	return nil
}

func serviceStart() error {
	out, err := runSchtasks("/Run", "/TN", serviceTaskName)
	if err != nil {
		return err
	}
	printNonEmpty(out)
	fmt.Printf("started scheduled task %q\n", serviceTaskName)
	return nil
}

func serviceStop(ignoreNotRunning bool) error {
	out, err := runSchtasks("/End", "/TN", serviceTaskName)
	if err != nil {
		if ignoreNotRunning && isTaskNotRunning(out) {
			return nil
		}
		return err
	}
	printNonEmpty(out)
	fmt.Printf("stopped scheduled task %q\n", serviceTaskName)
	return nil
}

func serviceRestart() error {
	if err := serviceStop(true); err != nil {
		return err
	}
	return serviceStart()
}

func serviceStatus() error {
	fmt.Printf("task:   %s\n", serviceTaskName)
	fmt.Printf("binary: %s\n", mustServiceExePath())
	fmt.Printf("config: %s\n", mustServiceConfigPath())
	fmt.Printf("log:    %s\n", mustServiceLogPath())
	fmt.Printf("state:  %s\n\n", mustServiceStatePath())
	out, err := runSchtasks("/Query", "/TN", serviceTaskName, "/FO", "LIST", "/V")
	if err != nil {
		if isTaskNotFound(out) {
			fmt.Printf("scheduled task %q is not installed\n", serviceTaskName)
			return nil
		}
		return err
	}
	printNonEmpty(out)
	return nil
}

func runSchtasks(args ...string) (string, error) {
	cmd := exec.Command("schtasks.exe", args...)
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil {
		return text, fmt.Errorf("schtasks %s failed: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(text))
	}
	return text, nil
}

func printNonEmpty(text string) {
	if trimmed := strings.TrimSpace(text); trimmed != "" {
		fmt.Println(trimmed)
	}
}

func isTaskNotRunning(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "not currently running") ||
		strings.Contains(lower, "not running") ||
		strings.Contains(lower, "cannot stop")
}

func isTaskNotFound(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "cannot find the file") ||
		strings.Contains(lower, "does not exist") ||
		strings.Contains(lower, "cannot find")
}

func servicePathWarnings(exePath, configPath string) []string {
	var warnings []string
	if _, err := os.Stat(exePath); errors.Is(err, os.ErrNotExist) {
		warnings = append(warnings, fmt.Sprintf("binary does not exist yet at %s", exePath))
	}
	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		warnings = append(warnings, fmt.Sprintf("config does not exist yet at %s", configPath))
	}
	return warnings
}

func mustServiceExePath() string {
	return filepath.Join(mustLocalAppData(), "Dexgram", "dexgram.exe")
}

func mustServiceConfigPath() string {
	return filepath.Join(mustRoamingAppData(), "Dexgram", "dexgram.toml")
}

func mustServiceLogPath() string {
	return filepath.Join(mustRoamingAppData(), "Dexgram", "dexgram.log")
}

func mustServiceStatePath() string {
	return filepath.Join(mustRoamingAppData(), "Dexgram", "dexgram.db")
}

func mustLocalAppData() string {
	if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
		return dir
	}
	dir, err := os.UserCacheDir()
	if err == nil && dir != "" {
		return dir
	}
	return filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
}

func mustRoamingAppData() string {
	if dir := os.Getenv("APPDATA"); dir != "" {
		return dir
	}
	dir, err := os.UserConfigDir()
	if err == nil && dir != "" {
		return dir
	}
	return filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
}
