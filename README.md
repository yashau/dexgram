# Dexgram

[![CI](https://img.shields.io/github/actions/workflow/status/yashau/dexgram/ci.yml?branch=main&style=for-the-badge&label=CI)](https://github.com/yashau/dexgram/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/yashau/dexgram?style=for-the-badge)](https://github.com/yashau/dexgram/releases)
[![License](https://img.shields.io/github/license/yashau/dexgram?style=for-the-badge)](LICENSE)
![Windows Only](https://img.shields.io/badge/Windows-Only-0078D4?style=for-the-badge&logo=windows11&logoColor=white)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev/)

Codex, wired straight into Telegram topics.

Dexgram is a Windows-only bridge that lets a Telegram bot drive real Codex
sessions. Each Telegram topic can become a resumable Codex thread, a
project-bound workspace, a side quest fork, or an attached session you already
started in Codex.

It ships as a native Windows binary. No local web app to babysit, no Node
runtime, no npm install, no surprise dependency pile just to send a prompt from
your phone.

It does not fake this by scraping a terminal. Dexgram talks to the Codex
app-server, so Telegram gets the good stuff: assistant text, live run logs, tool
activity, file edits, approvals, input requests, queued-message controls, and
final answers. Your phone becomes a command deck for the Codex sessions already
living on your Windows machine.

## Why Dexgram Hits Different

- **One native binary**: no Node runtime, no npm install, no local dashboard
  stack. Download `dexgram.exe`, configure your bot, and run it beside Codex.
- **Telegram is the interface**: stable push notifications, threaded chats,
  groups, DMs, search, files, photos, and mobile apps are already there.
  Dexgram only has to be the bridge.
- **No network gymnastics**: remote coding tools often make you care about
  line of sight, VPNs, Tailscale, port forwarding, relay servers, or a big
  custom dashboard. Telegram already solved reliable mobile messaging.
- **Still useful after Codex mobile**: Codex is now in preview inside the
  ChatGPT mobile app, but it still depends on connected hosts and early reports
  make the experience sound rough around the edges. Dexgram stays deliberately
  simple: Telegram topic in, Codex session out.
- **Topic-native Codex**: one Telegram topic maps to one Codex thread, with
  `/sessions [query]` for attaching existing sessions, `/new` for fresh chats,
  `/project` for project-bound work, and `/sync [limit]` for bounded history
  sync.
- **Real side quests**: `/side` and `/btw` create native Codex thread forks in
  fresh Telegram topics, keeping the same project and cwd while the tangent runs
  independently.
- **Live, controllable runs**: Telegram mirrors assistant text, run logs, tool
  activity, file edits, final answers, approvals, and input prompts. Messages
  sent while Codex is busy queue locally with steer/delete controls.
- **Local context without drama**: attachments, local file links, goals, Plan
  Mode, model selection, reasoning effort, hot reload, pairing codes, and the
  Windows login task all stay inside the small bridge.

## Windows Only

Dexgram is intentionally Windows-only. Codex is local, the app-server is
local, and the bridge is happiest when it can live beside them.

Service mode uses a current-user Task Scheduler login task, falling back to the
per-user Startup folder if Task Scheduler refuses. It is not a Windows Service;
it runs as you, because Codex runs as you.

Because yes, Windows gets nice things too.

## Requirements

- Windows
- Codex installed and signed in
- A Telegram bot token from `@BotFather`
- Telegram threaded topics enabled for the bot

In `@BotFather`, enable:

```text
Bot Settings -> Threads Settings -> Threaded Mode
```

## Setup

### Option 1: Install The Latest Release

Run the installer from PowerShell:

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

Start Dexgram in the foreground:

```powershell
dexgram
```

Or install the background login task so it starts when you sign in:

```powershell
dexgram service install
dexgram service start
```

Update later from PowerShell:

```powershell
dexgram update
```

You can also update from Telegram with `/update`. Dexgram announces before it
restarts and again after it comes back, which is exactly the kind of civilized
behavior a bridge should have.

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
the same Codex environment.

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

- `/sessions [query]` opens the session browser. Dexgram lists projects first,
  then paginated Codex threads inside the selected project. Attach one to the
  current Telegram topic and Dexgram syncs the most recent 25 rendered history
  messages by default.
- `/new [title]` creates a new topic for a one-off Codex chat.
- `/new project query: title` creates a new topic already bound to a matched
  Codex project.
- `/side [message]` or `/btw [message]` forks the current Codex chat into a
  prefixed side topic.
  If `message` is present, Dexgram starts it in the new side topic immediately.
  The source topic must already be an active registered Codex thread.
- `/project <project name>` binds a new topic to a Codex project before
  the first prompt. Ambiguous matches get inline selection buttons.
- `/status` shows the topic mapping, project/cwd, and active turn state.
- `/sync [limit]` mirrors completed Codex turns that have not been synced yet.
  It defaults to 1 turn, caps at 5 turns, and truncates oversized historical
  output.
- `/update` updates Dexgram and restarts the bridge.
- `/steer <message>` steers the currently active Codex turn.
- `/goal <objective>` sets the native Codex goal for the topic.
- `/plan <message>` starts a Codex Plan Mode turn.
- `/settings` shows Telegram-started Plan Mode settings.
- `/model [model-id|auto]` chooses the Plan Mode model.
- `/effort [auto|minimal|low|medium|high|xhigh]` chooses reasoning effort.
- `/stop` or `/cancel` interrupts the active Codex turn.

## How Chats Run

On the first prompt in a Telegram topic that has no project or Codex thread,
Dexgram asks whether to resume an existing session, start a new chat, or set a
project first.

- **Resume a session** opens the `/sessions` browser, attaches the selected
  Codex thread to the topic, syncs recent history, then submits your waiting
  message.
- **Start new chat** creates a fresh Codex thread for that topic.
- **Set project first** lets you bind the topic with `/project` before sending
  work to Codex.

Once a choice creates or attaches a Codex thread, Dexgram saves the mapping by
Telegram `chat_id` and `message_thread_id`. Later messages in that topic reuse
the stored Codex thread.

### Existing Sessions

`/sessions [query]` is the fast lane back into work you already started. Dexgram
asks Codex for recent threads, groups them by project/cwd, and shows an inline
project tree. Pick a project, pick a thread, and the current Telegram topic is
attached to that Codex session.

Attach sync is intentionally bounded. Dexgram mirrors up to the most recent 25
rendered Telegram history messages so you get context without flooding the
topic. Manual `/sync [limit]` is also bounded: one completed turn by default,
five at most.

### Side Chats

Use `/side` or `/btw` inside an existing Codex topic to open a separate side
topic from the current context. Dexgram names side topics with a `↳N` prefix,
such as `↳1 Dexgram: auth flow`, while leaving the parent topic name unchanged.

Side chats are real Codex thread forks. They keep the parent topic's project and
cwd, can call tools, ask for approvals, run commands, edit files, and continue
independently after the fork. You can create multiple side chats from the same
parent topic; each one gets the next number.

`/side` and `/btw` are guarded: they only work after the source topic has
already started and registered a Codex thread. A fresh `/new` topic or
project-only topic needs its first normal Codex message before it can be forked.
If the parent thread is idle, Dexgram resumes it first and only creates the
Telegram side topic after Codex accepts the fork.

You can also include the first side-chat prompt in the command:

```text
/side check whether the auth refactor missed tests
```

Dexgram creates the side topic and starts that prompt there immediately.

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

Queued user messages get two inline buttons:

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

- Dexgram can attach existing Codex sessions exposed by Codex app-server, but it
  does not bulk-import your entire Codex history into Telegram.
- Attach sync is capped at the most recent 25 rendered Telegram messages.
- Manual `/sync` is intentionally capped and truncated so one command cannot
  flood a topic with a huge historical transcript.
- Dexgram is Windows-only by design.

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
