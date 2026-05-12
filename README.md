# Dexgram

Dexgram is a Windows Telegram bridge for Codex Desktop. It runs as a single
Windows binary, listens to a private Telegram bot chat with threaded topics
enabled, and maps each Telegram topic to a Codex thread.

Unlike most other Codex-to-Telegram bridges, Dexgram is specifically built around
Codex Desktop and Codex app-server threads. Telegram chats created through
Dexgram are registered in Codex Desktop's chat history, so you can start or
continue a conversation from Telegram and then pick it up again in Codex
Desktop, or go the other way when the same thread is available there.

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
- `/sync` mirrors completed Codex turns that have not been synced yet.
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

## Queued Messages

If you send another Telegram message while Codex is already working in that
topic, Dexgram still submits it to Codex. Codex queues it natively, and Dexgram
replies to the queued Telegram message with two buttons:

- `Steer` merges that queued input into the currently active Codex turn, then
  deletes the queued turn.
- `Delete queued` cancels the queued Codex turn without steering it.

The `/steer <message>` command is the text-command version of steering: it sends
that message directly into the active Codex turn.

## Files And Attachments

- Photos and image documents go to Codex as local images.
- Other documents are downloaded and sent to Codex by absolute path.
- Text plus files are submitted together.
- File-only messages are staged for the next text prompt.
- Queued prompts can include files too.
- Codex-created files are uploaded back to Telegram when Dexgram sees them.

## License

MIT. See [LICENSE](./LICENSE).
