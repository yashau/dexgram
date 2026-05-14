# Dexgram

[![CI](https://img.shields.io/github/actions/workflow/status/yashau/dexgram/ci.yml?branch=main&style=for-the-badge&label=CI)](https://github.com/yashau/dexgram/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/yashau/dexgram?style=for-the-badge)](https://github.com/yashau/dexgram/releases)
[![License](https://img.shields.io/github/license/yashau/dexgram?style=for-the-badge)](LICENSE)
![Windows Only](https://img.shields.io/badge/Windows-Only-0078D4?style=for-the-badge&logo=windows11&logoColor=white)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev/)

Dexgram is a Windows Telegram bridge for Desktop. It runs as a single
Windows binary, listens to a private Telegram bot chat with threaded topics
enabled, and maps each Telegram topic to a Codex thread.

Unlike most other Codex-to-Telegram bridges, Dexgram talks directly to
Desktop's app-server instead of shelling out through the Codex CLI. That is the
important difference: Telegram gets the rich Codex stream Desktop sees, with
assistant messages, live run-log updates, tool activity, file edits, final
answers, and interruption support.

## Features

- Creates Codex chats from Telegram topics, with each topic mapped to a
  resumable Codex app-server thread.
- Keeps Dexgram-created chats visible in Desktop history so conversations can
  move between Telegram and the desktop app.
- Supports project-bound chats with fuzzy Desktop project matching and
  inline selection buttons for ambiguous matches.
- Creates dated one-off workspaces for projectless chats, matching Desktop's
  "Don't work in a project" flow.
- Mirrors Codex progress back to Telegram with initial replies, live run-log
  updates, final answers, and a Stop button for active turns.
- Queues new Telegram messages while Codex is busy, with inline controls to
  steer queued items into the active turn or delete them before they run.
- Supports native Codex goals from Telegram with `/goal <objective>`.
- Starts true Codex Plan Mode turns from Telegram with `/plan <message>`, and
  stores Telegram-selected model/reasoning defaults for Plan Mode.
- Downloads Telegram photos and documents for Codex prompts, stages
  attachment-only messages, and can upload final-answer local file links back
  to Telegram when enabled.
- Installs as a current-user Windows login task, with a Startup-folder fallback
  when Task Scheduler is unavailable.

## Windows Only

Dexgram is designed for Windows. Its service mode uses a current-user Windows
Task Scheduler login task, with a Startup-folder fallback if Task Scheduler
denies access.

Because, apparently, macOS is not the only desktop allowed to receive nice
things.

## Requirements

- Windows
- Desktop installed and signed in
- A Telegram bot token from `@BotFather`
- Telegram threaded topics enabled for the bot

In `@BotFather`, enable:

```text
Bot Settings -> Threads Settings -> Threaded Mode
```

## Development Checks

Run the same checks used by GitHub Actions:

```powershell
.\scripts\check.ps1
```

The script uses the active Go toolchain, installs the pinned `golangci-lint`
version if needed, then runs gofmt verification, tests, coverage, and lint.

## Releases

`cmd/dexgram/versioninfo.json` is the source of truth for the Dexgram version.
`go generate ./cmd/dexgram` derives `cmd/dexgram/version.go` and the Windows
resource from that metadata.

On pushes to `main`, GitHub Actions runs the shared check first. If it passes
and no GitHub release exists for `v<version>`, CI builds `dexgram.exe`, writes a
SHA-256 checksum, and publishes both files to a new release.

## Setup

### Option 1: Install The Latest Release

Run the one-line installer:

```powershell
irm https://raw.githubusercontent.com/yashau/dexgram/main/install.ps1 | iex
```

The installer downloads the latest GitHub release, puts `dexgram.exe` under
`%LOCALAPPDATA%\Dexgram`, adds that folder to your user `PATH`, creates the
Dexgram config/log/state directories, and starts onboarding from the installed
`dexgram.exe` path.

After onboarding, validate the setup with the command printed by the installer.

Update later with:

```powershell
dexgram update
```

You can also update from Telegram with `/update`. Dexgram posts before it
restarts and posts again in the same topic after the updated bridge comes back.

Print the installed Dexgram version with:

```powershell
dexgram version
```

After a successful update, the installer starts the Dexgram service again from
the newly installed binary.

### Option 2: Manual Setup

If you already have `dexgram.exe`, run onboarding yourself:

```powershell
.\dexgram.exe onboard
```

You can also keep a local config beside the binary:

```powershell
Copy-Item .\dexgram.example.toml .\dexgram.toml
.\dexgram.exe onboard
```

At minimum, Dexgram needs:

```toml
[telegram]
bot_token = "123456789:replace-me"
chat_ids = []
```

Start Dexgram and send the bot a DM from any chat that is not in `chat_ids`.
Dexgram will not send unauthorized chats to Codex; it will reply with the exact
command to run, for example:

```powershell
.\dexgram.exe telegram chatid add <logged_chat_id>
```

The shorter alias works too:

```powershell
.\dexgram.exe tg id add <logged_chat_id>
```

For unauthorized chats, Codex prompts and slash commands stay disabled. If
`chat_ids` is empty, every chat is unauthorized and receives only setup
instructions. Once a chat is configured, commands are registered only for
configured chats. Negative Telegram chat IDs are accepted.

To remove one registered chat or clear all registered chats later:

```powershell
.\dexgram.exe telegram chatid del <chat_id>
.\dexgram.exe telegram chatid clear
```

Validate the setup:

```powershell
.\dexgram.exe -check
```

Start Dexgram:

```powershell
.\dexgram.exe
```

## Service Mode

Dexgram can install itself as a user-login background process:

```powershell
.\dexgram.exe service install
.\dexgram.exe service start
.\dexgram.exe service status
```

This is not a Windows Service. It runs in the signed-in user's context so it can
talk to the same Desktop environment. Dexgram tries a current-user
scheduled task first; if Windows denies that, it installs a per-user Startup
folder fallback.

`service status` prints the Dexgram version, fixed paths, runtime process
state, and Task Scheduler status.

Service paths:

```text
Binary: %LOCALAPPDATA%\Dexgram\dexgram.exe
Config: %APPDATA%\Dexgram\dexgram.toml
Logs:   %APPDATA%\Dexgram\dexgram.log
State:  %APPDATA%\Dexgram\dexgram.db
```

The service log keeps the newest 5000 lines so background runs do not grow the
log indefinitely.

## Telegram Commands

- `/new [title]` creates a new Telegram topic for a Codex chat.
- `/project <project name>` binds a new topic to a Desktop project before
  the first prompt. Project matching is fuzzy, so partial names usually work;
  if multiple projects match, Dexgram shows inline selection buttons.
- `/status` shows the topic's Dexgram mapping and active turn state.
- `/sync` mirrors new completed Codex turns for chats Dexgram already created.
- `/update` updates Dexgram and restarts the bridge.
- `/steer <message>` steers the currently active Codex turn.
- `/goal <objective>` sets the native Codex goal for the topic. As of writing,
  goals must be enabled in your Codex config first:
  `%USERPROFILE%\.codex\config.toml` with `[features]` and `goals = true`.
- `/plan <message>` starts a Codex Plan Mode turn in the current topic.
- `/settings` shows the model and reasoning effort used for Telegram-started
  Plan Mode turns.
- `/model [model-id|auto]` chooses the model for Telegram-started Plan Mode
  turns. Without an argument it opens an inline selection menu.
- `/effort [auto|minimal|low|medium|high|xhigh]` chooses the reasoning effort
  for Telegram-started Plan Mode turns. Without an argument it opens an inline
  selection menu.
- `/stop` or `/cancel` interrupts the active Codex turn.

## How Chats Run

On the first prompt in a Telegram topic, Dexgram starts a Codex thread and
saves the mapping by Telegram `chat_id` and `message_thread_id`. Later messages
in that topic use the stored Codex thread id. If no project is selected,
Dexgram creates a one-off workspace similar to Desktop's "Don't work in a
project" flow under:

```text
%USERPROFILE%\Documents\Codex\YYYY-MM-DD\chat-title
```

Each Codex turn is mirrored back into Telegram as a small set of messages:

- An initial assistant or plan message, when Codex sends one.
- One live run-log message for commands, tools, file edits, searches, and media.
- The final assistant answer.

Live status and run-log messages are sent silently to keep Telegram quiet while
Codex works. Final answers, approval prompts, and user-input prompts use normal
Telegram notifications because they are the messages that usually need attention.

The active turn status message includes a `Stop` button. It does the same job
as `/stop` or `/cancel`: Dexgram interrupts the current Codex turn for that
topic.

## Limitations

- Dexgram does not import arbitrary Desktop chats into Telegram.
- `/sync` only works for chats Dexgram already created and mapped.
- Desktop work in a mapped chat can be mirrored back with `/sync`; unrelated
  Desktop chats will not appear in Telegram.

## Queued Messages

If you send another Telegram message while Codex is already working in that
topic, Dexgram keeps it queued locally and submits it to Codex when earlier
work finishes. Dexgram replies to the queued Telegram message with two buttons:

- `Steer` merges that queued input into the currently active Codex turn, then
  deletes the local queued item.
- `Delete` deletes the local queued item before it is submitted to Codex.

The `/steer <message>` command is the text-command version of steering: it sends
that message directly into the active Codex turn.

## Files And Attachments

Dexgram downloads Telegram photos, image documents, and regular documents to
local files before sending them to Codex. Images are passed as local image
inputs; other documents are included by absolute path.

Messages that include both text and files are sent to Codex as one prompt. If
you send files without text, Dexgram stages them and attaches them to the next
message in that chat. The same attachment handling applies when a prompt is
queued behind an active Codex turn.

Dexgram does not upload files reported by intermediate tool calls, file edits,
or image-generation events while Codex is working. By default, final answers are
text-only. To upload files explicitly linked by the final assistant answer, opt
in with:

```toml
[telegram]
upload_final_answer_files = true
```

When enabled, images are sent as photos and everything else is sent as a
document.

## License

MIT. See [LICENSE](./LICENSE).
