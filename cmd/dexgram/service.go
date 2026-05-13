package main

import (
	"encoding/json"
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
The service log keeps the newest %[7]d lines.

`, exe, mustServiceExePath(), mustServiceConfigPath(), mustServiceLogPath(), mustServiceStatePath(), serviceTaskName, logFileMaxLines)
}

func serviceInstall() error {
	exePath := mustServiceExePath()
	configPath := mustServiceConfigPath()
	logPath := mustServiceLogPath()
	taskUser := currentTaskUser()
	if err := os.MkdirAll(filepath.Dir(exePath), 0o755); err != nil {
		return fmt.Errorf("create binary directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	warnings := servicePathWarnings(exePath, configPath)
	taskRun := fmt.Sprintf(`"%s" -config "%s" -log "%s"`, exePath, configPath, logPath)
	args := []string{"/Create", "/TN", serviceTaskName, "/SC", "ONLOGON", "/TR", taskRun, "/RL", "LIMITED", "/F"}
	if taskUser != "" {
		args = append(args, "/RU", taskUser, "/IT")
	}
	out, err := runSchtasks(args...)
	if err != nil {
		if !isAccessDenied(out) {
			return err
		}
		if fallbackErr := installStartupFallback(taskRun); fallbackErr != nil {
			return fmt.Errorf("%w\nstartup fallback failed: %v", err, fallbackErr)
		}
		fmt.Printf("installed Startup login item: %s\n", mustStartupEntryPath())
		if startErr := startDexgramDirect(); startErr != nil {
			return startErr
		}
		for _, warning := range warnings {
			fmt.Fprintln(os.Stderr, "warning:", warning)
		}
		fmt.Printf("binary: %s\n", exePath)
		fmt.Printf("config: %s\n", configPath)
		fmt.Printf("log:    %s\n", logPath)
		return nil
	}
	printNonEmpty(out)
	_ = removeStartupFallback()
	for _, warning := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", warning)
	}
	fmt.Printf("installed user-login task %q\n", serviceTaskName)
	if taskUser != "" {
		fmt.Printf("user:   %s\n", taskUser)
	}
	fmt.Printf("binary: %s\n", exePath)
	fmt.Printf("config: %s\n", configPath)
	fmt.Printf("log:    %s\n", logPath)
	return nil
}

func serviceUninstall() error {
	_ = serviceStop(true)
	out, err := runSchtasks("/Delete", "/TN", serviceTaskName, "/F")
	if err != nil {
		if !isTaskNotFound(out) {
			return err
		}
	} else {
		printNonEmpty(out)
		fmt.Printf("removed scheduled task %q\n", serviceTaskName)
	}
	if err := removeStartupFallback(); err != nil {
		return err
	}
	return nil
}

func serviceStart() error {
	out, err := runSchtasks("/Run", "/TN", serviceTaskName)
	if err != nil {
		if !isTaskNotFound(out) {
			return err
		}
		if !startupFallbackInstalled() {
			return fmt.Errorf("service is not installed; run %s service install", filepath.Base(os.Args[0]))
		}
		return startDexgramDirect()
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
	printServiceStatusHeader(os.Stdout)
	printDexgramRuntimeStatus()
	out, err := runSchtasks("/Query", "/TN", serviceTaskName, "/FO", "LIST", "/V")
	if err != nil {
		if isTaskNotFound(out) {
			fmt.Printf("scheduled task %q is not installed\n", serviceTaskName)
		} else {
			return err
		}
	} else {
		printNonEmpty(out)
	}
	if startupFallbackInstalled() {
		fmt.Printf("Startup fallback is installed at %s\n", mustStartupEntryPath())
	} else {
		fmt.Println("Startup fallback is not installed")
	}
	return nil
}

func printServiceStatusHeader(w io.Writer) {
	fmt.Fprintf(w, "task:   %s\n", serviceTaskName)
	fmt.Fprintf(w, "version: %s\n", appVersion)
	fmt.Fprintf(w, "binary: %s\n", mustServiceExePath())
	fmt.Fprintf(w, "config: %s\n", mustServiceConfigPath())
	fmt.Fprintf(w, "log:    %s\n", mustServiceLogPath())
	fmt.Fprintf(w, "state:  %s\n", mustServiceStatePath())
	fmt.Fprintf(w, "startup fallback: %s\n\n", mustStartupEntryPath())
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

func isAccessDenied(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "access is denied") ||
		strings.Contains(lower, "access denied")
}

func currentTaskUser() string {
	user := strings.TrimSpace(os.Getenv("USERNAME"))
	if user == "" {
		return ""
	}
	domain := strings.TrimSpace(os.Getenv("USERDOMAIN"))
	if domain == "" || strings.Contains(user, `\`) {
		return user
	}
	return domain + `\` + user
}

func installStartupFallback(command string) error {
	path := mustStartupEntryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create startup directory: %w", err)
	}
	content := "@echo off\r\n" +
		"rem Dexgram user-login startup fallback\r\n" +
		"start \"\" /min cmd.exe /d /c " + quoteCmdArg(command) + "\r\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write startup fallback: %w", err)
	}
	return nil
}

func removeStartupFallback() error {
	path := mustStartupEntryPath()
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("remove Startup fallback: %w", err)
	}
	fmt.Printf("removed Startup fallback %q\n", path)
	return nil
}

func startupFallbackInstalled() bool {
	_, err := os.Stat(mustStartupEntryPath())
	return err == nil
}

func startDexgramDirect() error {
	cmd := exec.Command(mustServiceExePath(), "-config", mustServiceConfigPath(), "-log", mustServiceLogPath())
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start Dexgram: %w", err)
	}
	fmt.Printf("started Dexgram process pid=%d\n", cmd.Process.Pid)
	return cmd.Process.Release()
}

func quoteCmdArg(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func printDexgramRuntimeStatus() {
	lines, err := runningDexgramProcesses()
	if err != nil {
		fmt.Printf("runtime: unknown (%v)\n\n", err)
		return
	}
	if len(lines) == 0 {
		fmt.Println("runtime: stopped")
		fmt.Println()
		return
	}
	fmt.Println("runtime: running")
	for _, line := range lines {
		fmt.Println("  " + line)
	}
	fmt.Println()
}

func runningDexgramProcesses() ([]string, error) {
	script := fmt.Sprintf(`Get-Process -Name dexgram -ErrorAction SilentlyContinue | Where-Object { $_.Id -ne %d } | Select-Object Id,Path | ConvertTo-Json -Compress`, os.Getpid())
	out, err := exec.Command("powershell.exe", "-NoProfile", "-Command", script).CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil && text != "" {
		return nil, err
	}
	if text == "" {
		return nil, nil
	}
	var raw any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, err
	}
	var records []map[string]any
	switch value := raw.(type) {
	case []any:
		for _, item := range value {
			if record, ok := item.(map[string]any); ok {
				records = append(records, record)
			}
		}
	case map[string]any:
		records = append(records, value)
	}
	var lines []string
	for _, record := range records {
		id := fmt.Sprint(record["Id"])
		path := strings.TrimSpace(fmt.Sprint(record["Path"]))
		if path == "" || path == "<nil>" {
			path = "path unavailable"
		}
		lines = append(lines, "pid="+id+" path="+path)
	}
	return lines, nil
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

func mustStartupEntryPath() string {
	return filepath.Join(mustRoamingAppData(), "Microsoft", "Windows", "Start Menu", "Programs", "Startup", serviceTaskName+".cmd")
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
