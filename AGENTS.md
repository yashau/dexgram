# AGENTS.md

Guidance for agents working on Dexgram.

## Project Overview

Dexgram is a Windows-only Telegram bridge for Codex Desktop. It runs as a Go
binary, listens to a private Telegram bot chat with threaded topics enabled,
and maps Telegram topics to Codex app-server threads.

The main command lives in `cmd/dexgram`. Shared packages live under `internal/`:

- `internal/codex`: Codex app-server JSON-RPC client.
- `internal/codexprojects`: Codex Desktop project discovery.
- `internal/codexstate`: projectless workspace helpers.
- `internal/config`: TOML config loading/defaults.
- `internal/state`: SQLite-backed mapping and sync state.

Keep changes small and aligned with the current layout. Prefer local helpers and
existing package boundaries over adding new framework-like abstractions.

## Development Commands

Run these before handing work back:

```powershell
gofmt -w .
go test ./...
golangci-lint run ./...
```

If `golangci-lint` is not installed, note that clearly in your final response.
Do not silently skip it.

To build the Windows binary:

```powershell
go build -o dexgram.exe .\cmd\dexgram
```

## Windows Resource and Binary Version

Dexgram embeds Windows file metadata through `goversioninfo`, matching the
pattern used by the Viberator project.

The entrypoint contains:

```go
//go:generate goversioninfo -64 -o resource.syso
```

The metadata source is:

```text
cmd/dexgram/versioninfo.json
```

The generated resource file is:

```text
cmd/dexgram/resource.syso
```

To change the binary version or author metadata:

1. Edit `cmd/dexgram/versioninfo.json`.
2. Update both numeric versions under `FixedFileInfo`:
   `FileVersion` and `ProductVersion`.
3. Update matching strings under `StringFileInfo`:
   `FileVersion` and `ProductVersion`.
4. Update author/copyright fields there as needed.
5. Regenerate the resource:

```powershell
go generate ./cmd/dexgram
```

Current expected metadata is version `0.1.7` and author/copyright
`2026 Yashau`.

If `goversioninfo` is missing, install it with:

```powershell
go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest
```

On Windows, ensure `%USERPROFILE%\go\bin` is on `PATH` before running
`go generate`.

## Runtime Notes

Dexgram is intentionally Windows-specific:

- Service mode prefers a current-user Windows Task Scheduler login task, falls
  back to a per-user Startup folder item, and is not a Windows Service.
- Default paths use `%APPDATA%` and `%LOCALAPPDATA%`.
- The Codex CLI is auto-discovered at
  `%LOCALAPPDATA%\OpenAI\Codex\bin\codex.exe`.

Avoid replacing this Windows behavior with cross-platform assumptions unless
the user explicitly asks for that.

## Editing Guidelines

- Preserve existing user configuration files such as `dexgram.toml`.
- Do not commit or depend on generated local binaries like `dexgram.exe`.
- Keep Telegram command behavior documented in `README.md` when user-facing
  commands change.
- Use structured parsing for TOML, JSON, and SQLite data instead of ad hoc
  string manipulation.
- Keep logs and errors useful for a user running Dexgram from PowerShell or as
  a background login task.

## Quick Smoke Checks

For config-only or documentation-only changes, `gofmt` may not alter anything,
but still run tests when Go behavior could be affected.

For changes touching Windows version metadata, run:

```powershell
go generate ./cmd/dexgram
go build -o "$env:TEMP\dexgram-version-test.exe" .\cmd\dexgram
```

Then inspect the built file with PowerShell:

```powershell
[System.Diagnostics.FileVersionInfo]::GetVersionInfo("$env:TEMP\dexgram-version-test.exe")
```
