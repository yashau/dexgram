package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartupFallbackContentUsesCmdCompatibleQuotes(t *testing.T) {
	command := `"C:\Users\Yashau\AppData\Local\Dexgram\dexgram.exe" -config "C:\Users\Yashau\AppData\Roaming\Dexgram\dexgram.toml" -log "C:\Users\Yashau\AppData\Roaming\Dexgram\dexgram.log"`
	logPath := `C:\Users\Yashau\AppData\Roaming\Dexgram\dexgram.log`

	got := startupFallbackContent(command, logPath)

	if strings.Contains(got, `\"`) {
		t.Fatalf("startup fallback uses C-style quote escaping: %q", got)
	}
	wantLine := `start "" /min cmd.exe /d /c ""C:\Users\Yashau\AppData\Local\Dexgram\dexgram.exe" -config "C:\Users\Yashau\AppData\Roaming\Dexgram\dexgram.toml" -log "C:\Users\Yashau\AppData\Roaming\Dexgram\dexgram.log" >> "C:\Users\Yashau\AppData\Roaming\Dexgram\dexgram.log" 2>&1"`
	if !strings.Contains(got, wantLine+"\r\n") {
		t.Fatalf("startup fallback content = %q, want line %q", got, wantLine)
	}
}

func TestPrintServiceHelpIncludesTaskAndPaths(t *testing.T) {
	var out bytes.Buffer
	printServiceHelp(&out, "dexgram.exe")
	text := out.String()
	for _, want := range []string{"dexgram.exe service install", serviceTaskName, "Startup folder fallback", "Binary:", "Config:", "Logs:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("service help missing %q: %q", want, text)
		}
	}
}

func TestRunServiceCommandHelpAndUnknownCommand(t *testing.T) {
	if err := runServiceCommand([]string{"help"}); err != nil {
		t.Fatalf("service help command: %v", err)
	}
	if err := runServiceCommand([]string{"bogus"}); err == nil {
		t.Fatal("expected unknown service command to fail")
	}
}

func TestQuoteCmdArgDoublesEmbeddedQuotes(t *testing.T) {
	got := quoteCmdArg(`C:\Users\Name "Quoted"\dexgram.log`)
	want := `"C:\Users\Name ""Quoted""\dexgram.log"`
	if got != want {
		t.Fatalf("quoteCmdArg() = %q, want %q", got, want)
	}
}

func TestQuoteCmdCommandWrapsFullCommand(t *testing.T) {
	if got := quoteCmdCommand(`"C:\Program Files\Dexgram\dexgram.exe" -check`); got != `""C:\Program Files\Dexgram\dexgram.exe" -check"` {
		t.Fatalf("quoteCmdCommand() = %q", got)
	}
}

func TestQuotePowerShellStringEscapesSingleQuotes(t *testing.T) {
	got := quotePowerShellString(`C:\Users\O'Brien\AppData\Local\Dexgram\dexgram.exe`)
	want := `'C:\Users\O''Brien\AppData\Local\Dexgram\dexgram.exe'`
	if got != want {
		t.Fatalf("quotePowerShellString() = %q, want %q", got, want)
	}
}

func TestServiceCommandOutputClassifiers(t *testing.T) {
	if !isTaskNotRunning("ERROR: The system cannot stop the task because it is not currently running.") {
		t.Fatal("expected task-not-running output to be recognized")
	}
	if !isTaskNotFound("ERROR: The system cannot find the file specified.") {
		t.Fatal("expected task-not-found output to be recognized")
	}
	if !isAccessDenied("ERROR: Access is denied.") {
		t.Fatal("expected access-denied output to be recognized")
	}
	if isTaskNotFound("SUCCESS: The scheduled task exists.") {
		t.Fatal("did not expect success output to look missing")
	}
}

func TestServiceStopFallsBackWhenTaskIsMissingAndStartupFallbackIsInstalled(t *testing.T) {
	roaming := filepath.Join(t.TempDir(), "Roaming")
	t.Setenv("APPDATA", roaming)
	if err := installStartupFallback(`"C:\Dexgram\dexgram.exe"`, `C:\Dexgram\dexgram.log`); err != nil {
		t.Fatalf("install startup fallback: %v", err)
	}

	oldRunSchtasks := runSchtasks
	oldStopDirect := stopDexgramDirectProcessesFunc
	defer func() {
		runSchtasks = oldRunSchtasks
		stopDexgramDirectProcessesFunc = oldStopDirect
	}()

	var gotArgs []string
	runSchtasks = func(args ...string) (string, error) {
		gotArgs = append([]string(nil), args...)
		return "ERROR: The system cannot find the file specified.", errors.New("exit status 1")
	}
	stopDirectCalled := false
	stopDexgramDirectProcessesFunc = func() ([]string, error) {
		stopDirectCalled = true
		return []string{`pid=123 path=C:\Dexgram\dexgram.exe`}, nil
	}

	if err := serviceStop(false); err != nil {
		t.Fatalf("serviceStop() = %v", err)
	}
	if !stopDirectCalled {
		t.Fatal("expected Startup fallback process stop path to be used")
	}
	wantArgs := []string{"/End", "/TN", serviceTaskName}
	if strings.Join(gotArgs, "|") != strings.Join(wantArgs, "|") {
		t.Fatalf("schtasks args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestServiceStopFallsBackWhenTaskIsNotRunningAndStartupFallbackIsInstalled(t *testing.T) {
	roaming := filepath.Join(t.TempDir(), "Roaming")
	t.Setenv("APPDATA", roaming)
	if err := installStartupFallback(`"C:\Dexgram\dexgram.exe"`, `C:\Dexgram\dexgram.log`); err != nil {
		t.Fatalf("install startup fallback: %v", err)
	}

	oldRunSchtasks := runSchtasks
	oldStopDirect := stopDexgramDirectProcessesFunc
	defer func() {
		runSchtasks = oldRunSchtasks
		stopDexgramDirectProcessesFunc = oldStopDirect
	}()

	runSchtasks = func(args ...string) (string, error) {
		return "ERROR: The system cannot stop the task because it is not currently running.", errors.New("exit status 1")
	}
	stopDirectCalled := false
	stopDexgramDirectProcessesFunc = func() ([]string, error) {
		stopDirectCalled = true
		return []string{`pid=123 path=C:\Dexgram\dexgram.exe`}, nil
	}

	if err := serviceStop(false); err != nil {
		t.Fatalf("serviceStop() = %v", err)
	}
	if !stopDirectCalled {
		t.Fatal("expected Startup fallback process stop path to be used")
	}
}

func TestServiceStopAlsoStopsStartupFallbackAfterScheduledTaskStops(t *testing.T) {
	roaming := filepath.Join(t.TempDir(), "Roaming")
	t.Setenv("APPDATA", roaming)
	if err := installStartupFallback(`"C:\Dexgram\dexgram.exe"`, `C:\Dexgram\dexgram.log`); err != nil {
		t.Fatalf("install startup fallback: %v", err)
	}

	oldRunSchtasks := runSchtasks
	oldStopDirect := stopDexgramDirectProcessesFunc
	defer func() {
		runSchtasks = oldRunSchtasks
		stopDexgramDirectProcessesFunc = oldStopDirect
	}()

	runSchtasks = func(args ...string) (string, error) {
		return "SUCCESS: The scheduled task was terminated.", nil
	}
	stopDirectCalled := false
	stopDexgramDirectProcessesFunc = func() ([]string, error) {
		stopDirectCalled = true
		return []string{`pid=123 path=C:\Dexgram\dexgram.exe`}, nil
	}

	if err := serviceStop(false); err != nil {
		t.Fatalf("serviceStop() = %v", err)
	}
	if !stopDirectCalled {
		t.Fatal("expected Startup fallback process stop path to be used")
	}
}

func TestServiceStartDoesNotDuplicateExistingStartupFallbackProcess(t *testing.T) {
	roaming := filepath.Join(t.TempDir(), "Roaming")
	t.Setenv("APPDATA", roaming)
	if err := installStartupFallback(`"C:\Dexgram\dexgram.exe"`, `C:\Dexgram\dexgram.log`); err != nil {
		t.Fatalf("install startup fallback: %v", err)
	}

	oldRunSchtasks := runSchtasks
	oldRunningDirect := runningDexgramDirectProcessesFunc
	defer func() {
		runSchtasks = oldRunSchtasks
		runningDexgramDirectProcessesFunc = oldRunningDirect
	}()

	runSchtasks = func(args ...string) (string, error) {
		return "ERROR: The system cannot find the file specified.", errors.New("exit status 1")
	}
	runningDexgramDirectProcessesFunc = func() ([]string, error) {
		return []string{`pid=123 path=C:\Dexgram\dexgram.exe`}, nil
	}

	if err := serviceStart(); err != nil {
		t.Fatalf("serviceStart() = %v", err)
	}
}

func TestCurrentTaskUserUsesDomainWhenAvailable(t *testing.T) {
	t.Setenv("USERNAME", "alice")
	t.Setenv("USERDOMAIN", "WORKSTATION")
	if got := currentTaskUser(); got != `WORKSTATION\alice` {
		t.Fatalf("current task user = %q", got)
	}

	t.Setenv("USERNAME", `DOMAIN\bob`)
	t.Setenv("USERDOMAIN", "IGNORED")
	if got := currentTaskUser(); got != `DOMAIN\bob` {
		t.Fatalf("domain-qualified user = %q", got)
	}

	t.Setenv("USERNAME", "")
	if got := currentTaskUser(); got != "" {
		t.Fatalf("empty user = %q", got)
	}
}

func TestServicePathsAndWarningsUseConfiguredAppData(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, "Local")
	roaming := filepath.Join(root, "Roaming")
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("APPDATA", roaming)

	exePath := mustServiceExePath()
	configPath := mustServiceConfigPath()
	if !strings.HasPrefix(exePath, local) {
		t.Fatalf("service exe path = %q", exePath)
	}
	if !strings.HasPrefix(configPath, roaming) {
		t.Fatalf("service config path = %q", configPath)
	}

	warnings := servicePathWarnings(exePath, configPath)
	if len(warnings) != 2 {
		t.Fatalf("warnings for missing paths = %#v", warnings)
	}
	if err := os.MkdirAll(filepath.Dir(exePath), 0o755); err != nil {
		t.Fatalf("create exe dir: %v", err)
	}
	if err := os.WriteFile(exePath, []byte("binary"), 0o600); err != nil {
		t.Fatalf("write exe: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("config"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if warnings := servicePathWarnings(exePath, configPath); len(warnings) != 0 {
		t.Fatalf("warnings for present paths = %#v", warnings)
	}
}

func TestStartupFallbackInstallStatusAndRemove(t *testing.T) {
	roaming := filepath.Join(t.TempDir(), "Roaming")
	t.Setenv("APPDATA", roaming)

	if startupFallbackInstalled() {
		t.Fatal("startup fallback should not exist before install")
	}
	if err := installStartupFallback(`"C:\Dexgram\dexgram.exe"`, `C:\Dexgram\dexgram.log`); err != nil {
		t.Fatalf("install startup fallback: %v", err)
	}
	if !startupFallbackInstalled() {
		t.Fatal("startup fallback should exist after install")
	}
	content, err := os.ReadFile(mustStartupEntryPath())
	if err != nil {
		t.Fatalf("read startup fallback: %v", err)
	}
	if !strings.Contains(string(content), "Dexgram user-login startup fallback") {
		t.Fatalf("unexpected startup fallback content: %q", content)
	}
	if err := removeStartupFallback(); err != nil {
		t.Fatalf("remove startup fallback: %v", err)
	}
	if startupFallbackInstalled() {
		t.Fatal("startup fallback should be removed")
	}
	if err := removeStartupFallback(); err != nil {
		t.Fatalf("remove missing startup fallback: %v", err)
	}
}
