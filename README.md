# Dexgram

Dexgram is a Windows Telegram bridge for Codex Desktop. It runs as a single
Windows binary, listens to a private Telegram bot chat with threaded topics
enabled, and maps each Telegram topic to a Codex thread.

Unlike most other Codex-to-Telegram bridges, Dexgram is specifically built around
Codex Desktop and Codex app-server threads. Telegram chats created through
Dexgram are registered in Codex Desktop's chat history, so you can start or
continue a conversation from Telegram and then pick it up again in Codex
Desktop.

## Features

- Creates Codex chats from Telegram topics, with each topic mapped to a
  resumable Codex app-server thread.
- Keeps Dexgram-created chats visible in Codex Desktop history so conversations
  can move between Telegram and the desktop app.
- Supports project-bound chats with fuzzy Codex Desktop project matching and
  inline selection buttons for ambiguous matches.
- Creates dated one-off workspaces for projectless chats, matching Codex
  Desktop's "Don't work in a project" flow.
- Mirrors Codex progress back to Telegram with initial replies, live run-log
  updates, final answers, and a Stop button for active turns.
- Queues new Telegram messages while Codex is busy, with inline controls to
  steer queued items into the active turn or delete them before they run.
- Supports native Codex goals from Telegram with `/goal <objective>`.
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
- Codex Desktop installed and signed in
- A Telegram bot token from `@BotFather`
- Telegram threaded topics enabled for the bot

In `@BotFather`, enable:

```text
Bot Settings -> Threads Settings -> Threaded Mode
```

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

After a successful update, the installer starts the Dexgram service again from
the newly installed binary.

### Option 2: Manual Setup

If you already have `dexgram.exe`, run onboarding yourself:

```powershell
.\dexgram.exe onboard
```

You can also copy the example config and edit it manually:

```powershell
Copy-Item .\dexgram.example.toml .\dexgram.toml
notepad .\dexgram.toml
```

At minimum, set:

```toml
[telegram]
bot_token = "123456789:replace-me"
chat_id = 0
```

Use `chat_id = 0` temporarily to discover your private chat id. Start Dexgram,
send the bot a DM, then copy the logged `chat_id` into the config.

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
```

This is not a Windows Service. It runs in the signed-in user's context so it can
talk to the same Codex Desktop environment. Dexgram tries a current-user
scheduled task first; if Windows denies that, it installs a per-user Startup
folder fallback.

Service paths:

```text
Binary: %LOCALAPPDATA%\Dexgram\dexgram.exe
Config: %APPDATA%\Dexgram\dexgram.toml
Logs:   %APPDATA%\Dexgram\dexgram.log
State:  %APPDATA%\Dexgram\dexgram.db
```

## Telegram Commands

- `/new [title]` creates a new Telegram topic for a Codex chat.
- `/project <project name>` binds a new topic to a Codex Desktop project before
  the first prompt. Project matching is fuzzy, so partial names usually work;
  if multiple projects match, Dexgram shows inline selection buttons.
- `/status` shows the topic's Dexgram mapping and active turn state.
- `/sync` mirrors new completed Codex turns for chats Dexgram already created.
- `/steer <message>` steers the currently active Codex turn.
- `/goal <objective>` sets the native Codex goal for the topic. As of writing,
  goals must be enabled in your Codex config first:
  `%USERPROFILE%\.codex\config.toml` with `[features]` and `goals = true`.
- `/stop` or `/cancel` interrupts the active Codex turn.

## How Chats Run

On the first prompt in a Telegram topic, Dexgram starts a Codex thread and
saves the mapping by Telegram `chat_id` and `message_thread_id`. Later messages
in that topic use the stored Codex thread id. If no project is selected,
Dexgram creates a one-off workspace similar to Codex Desktop's "Don't work in a
project" flow under:

```text
%USERPROFILE%\Documents\Codex\YYYY-MM-DD\chat-title
```

Each Codex turn is mirrored back into Telegram as a small set of messages:

- An initial assistant or plan message, when Codex sends one.
- One live run-log message for commands, tools, file edits, searches, and media.
- The final assistant answer.

The active turn status message includes a `Stop` button. It does the same job
as `/stop` or `/cancel`: Dexgram interrupts the current Codex turn for that
topic.

## Limitations

- Dexgram does not import arbitrary Codex Desktop chats into Telegram.
- `/sync` only works for chats Dexgram already created and mapped.
- Desktop work in a mapped chat can be mirrored back with `/sync`; unrelated
  Desktop chats will not appear in Telegram.

## Queued Messages

If you send another Telegram message while Codex is already working in that
topic, Dexgram still submits it to Codex. Codex queues it natively, and Dexgram
replies to the queued Telegram message with two buttons:

- `Steer` merges that queued input into the currently active Codex turn, then
  deletes the queued turn.
- `Delete` cancels the queued Codex turn without steering it.

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
