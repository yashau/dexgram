# Dexgram

[![CI](https://img.shields.io/github/actions/workflow/status/yashau/dexgram/ci.yml?branch=main&style=for-the-badge&label=CI)](https://github.com/yashau/dexgram/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/yashau/dexgram?style=for-the-badge)](https://github.com/yashau/dexgram/releases)
[![License](https://img.shields.io/github/license/yashau/dexgram?style=for-the-badge)](LICENSE)
![Windows Only](https://img.shields.io/badge/Windows-Only-0078D4?style=for-the-badge&logo=windows11&logoColor=white)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev/)

Dexgram is a Windows Telegram bridge for Codex Desktop. It runs as one binary,
listens through your Telegram bot, and maps Telegram topics to resumable Codex
threads.

Unlike most Codex-to-Telegram bridges, Dexgram talks to Codex app-server instead
of driving a plain CLI prompt. Telegram gets the rich stream: assistant text,
live run-log updates, tool activity, file edits, final answers, approvals,
input requests, and stop buttons. Tiny bridge, surprisingly roomy.

## Features

- Creates resumable Codex chats from Telegram topics.
- Keeps Dexgram-created chats visible in Codex Desktop history.
- Supports project-bound chats with fuzzy Desktop project matching.
- Creates dated one-off workspaces for projectless chats.
- Mirrors live Codex progress, final answers, approvals, and input prompts.
- Queues messages while Codex is busy, with buttons to steer or delete them.
- Supports `/goal`, `/plan`, model selection, and reasoning effort selection.
- Downloads Telegram photos and documents for Codex prompts.
- Can upload final-answer local file links back to Telegram when enabled.
- Authorizes Telegram chats by config, with short pairing codes for onboarding.
- Hot-reloads config changes, including `chat_ids`, Codex settings, and bot token.
- Runs as a current-user Windows login task, with a Startup-folder fallback.

## Windows Only

Dexgram is intentionally Windows-only. Service mode uses a current-user Task
Scheduler login task, falling back to the per-user Startup folder if Task
Scheduler refuses.

Because Windows gets nice things too.

## Requirements

- Windows
- Codex Desktop installed and signed in
- A Telegram bot token from `@BotFather`
- Telegram threaded topics enabled for the bot

In `@BotFather`, enable:

```text
Bot Settings -> Threads Settings -> Threaded Mode
```

## Setup

### Option 1: Install The Latest Release

Run the installer:

```powershell
irm https://raw.githubusercontent.com/yashau/dexgram/main/install.ps1 | iex
```

It downloads the latest release, installs `dexgram.exe` under
`%LOCALAPPDATA%\Dexgram`, adds that folder to your user `PATH`, creates the
config/log/state directories, and starts onboarding.

After onboarding, validate the setup:

```powershell
dexgram -check
```

Start Dexgram:

```powershell
dexgram
```

Or install the background login task:

```powershell
dexgram service install
dexgram service start
```

Update later with:

```powershell
dexgram update
```

You can also update from Telegram with `/update`. Dexgram announces before it
restarts and again after it comes back.

### Option 2: Manual Setup

If you already have `dexgram.exe`, run onboarding:

```powershell
.\dexgram.exe onboard
```

Onboarding asks for your BotFather token and Codex defaults. If no Telegram
chats are authorized yet, start Dexgram, then DM the bot or add it to a group
and send a message. The bot replies with the exact command to add that chat.

You can also keep a local config beside the binary:

```powershell
Copy-Item .\dexgram.example.toml .\dexgram.toml
.\dexgram.exe onboard
```

Minimum config:

```toml
[telegram]
bot_token = "123456789:replace-me"
chat_ids = []
```

## Pair A Telegram Chat

Unauthorized chats cannot use Codex, and Telegram slash commands are not
registered for them. They only get setup instructions.

Send any message from an unauthorized chat. Dexgram replies with a short-lived
pairing code and a ready-to-run command:

```powershell
dexgram telegram chatid add ABC-234
```

The code is case-insensitive and may be entered as `ABC-234` or `ABC234`. Direct
numeric chat IDs still work too, including negative group IDs:

```powershell
dexgram telegram chatid add -1001234567890
```

Aliases:

```powershell
dexgram tg id add ABC-234
dexgram telegram chatid del <chat_id>
dexgram telegram chatid clear
```

Dexgram hot-reloads the config, so adding or removing chats does not require a
restart.

To rotate the bot token without putting it in shell history:

```powershell
dexgram telegram token update
```

The running bridge reconnects with the new token after the config reloads.

## Service Mode

Dexgram can install itself as a user-login background process:

```powershell
dexgram service install
dexgram service start
dexgram service status
```

This is not a Windows Service. It runs as the signed-in user so it can talk to
the same Codex Desktop environment.

Service paths:

```text
Binary: %LOCALAPPDATA%\Dexgram\dexgram.exe
Config: %APPDATA%\Dexgram\dexgram.toml
Logs:   %APPDATA%\Dexgram\dexgram.log
State:  %APPDATA%\Dexgram\dexgram.db
```

The service log keeps the newest 5000 lines.

## Telegram Commands

Commands are registered only for authorized chats.

- `/new [title]` creates a new topic for a one-off Codex chat.
- `/new project query: title` creates a new topic pre-bound to a matched project.
- `/project <project name>` binds a new topic to a Codex Desktop project before
  the first prompt. Ambiguous matches get inline selection buttons.
- `/status` shows the topic mapping, project/cwd, and active turn state.
- `/sync` mirrors completed Codex turns that have not been synced yet.
- `/update` updates Dexgram and restarts the bridge.
- `/steer <message>` steers the currently active Codex turn.
- `/goal <objective>` sets the native Codex goal for the topic.
- `/plan <message>` starts a Codex Plan Mode turn.
- `/settings` shows Telegram-started Plan Mode settings.
- `/model [model-id|auto]` chooses the Plan Mode model.
- `/effort [auto|minimal|low|medium|high|xhigh]` chooses reasoning effort.
- `/stop` or `/cancel` interrupts the active Codex turn.

## How Chats Run

On the first prompt in a Telegram topic, Dexgram starts a Codex thread and saves
the mapping by Telegram `chat_id` and `message_thread_id`. Later messages in
that topic reuse the stored Codex thread.

Without `/project`, Dexgram creates a projectless workspace under:

```text
%USERPROFILE%\Documents\Codex\YYYY-MM-DD\chat-title
```

Each Codex turn appears in Telegram as:

- An initial assistant or plan message, when Codex sends one.
- One live run-log message for commands, tools, edits, searches, and media.
- The final assistant answer.

Live status and run-log messages are sent silently. Final answers, approval
prompts, and user-input prompts use normal Telegram notifications.

## Queued Messages

If you send a message while Codex is already working in that topic, Dexgram
queues it locally and submits it when earlier work finishes.

Queued messages get two buttons:

- `Steer` merges the queued input into the active Codex turn.
- `Delete` removes it before it is submitted.

`/steer <message>` does the same thing from text.

## Files And Attachments

Dexgram downloads Telegram photos, image documents, and regular documents to
local files before sending them to Codex. Images are passed as local image
inputs; other documents are included by absolute path.

Text plus files is sent as one prompt. Files without text are staged and
attached to the next message in that chat.

By default, final answers stay text-only. To upload local files explicitly
linked by the final answer:

```toml
[telegram]
upload_final_answer_files = true
```

Images are sent as photos. Everything else is sent as a document.

## Limitations

- Dexgram does not import arbitrary Codex Desktop chats into Telegram.
- `/sync` only works for chats Dexgram already created and mapped.
- Unrelated Desktop chats do not appear in Telegram.

## Development

Run the same checks used by GitHub Actions:

```powershell
.\scripts\check.ps1
```

The script verifies `gofmt`, runs tests and coverage, and runs the pinned
`golangci-lint`.

Version metadata lives in `cmd/dexgram/versioninfo.json`. Regenerate the Go and
Windows resource files with:

```powershell
go generate ./cmd/dexgram
```

On pushes to `main`, CI runs the shared check. If no GitHub release exists for
`v<version>`, CI builds `dexgram.exe`, writes a SHA-256 checksum, and publishes
both files.

## License

MIT. See [LICENSE](./LICENSE).
