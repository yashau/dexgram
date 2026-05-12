package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

const installScriptURL = "https://raw.githubusercontent.com/yashau/dexgram/main/install.ps1"

func runUpdateCommand() error {
	fmt.Println("Starting Dexgram updater...")
	script := fmt.Sprintf(
		"$env:UPDATE='true'; $env:DEXGRAM_UPDATE_PARENT_PID='%s'; irm %s | iex",
		strconv.Itoa(os.Getpid()),
		installScriptURL,
	)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
