package main

import (
	"flag"
	"fmt"
	"io"

	"dexgram/internal/state"
)

func printHelp(w io.Writer, exe string, fs *flag.FlagSet) {
	statePath, err := state.DefaultPath()
	if err != nil {
		statePath = "%APPDATA%\\Dexgram\\dexgram.db"
	}
	fmt.Fprintf(w, `Dexgram

  Telegram bridge for Codex Desktop / Codex app-server threads.
  It runs as a single Windows binary, listens to a private Telegram bot chat
  with threaded topics enabled, and maps each Telegram topic to one Codex
  thread.

Usage

  %[1]s [options]
  %[1]s -check
  %[1]s -config .\dexgram.toml
  %[1]s onboard
  %[1]s service <install|start|stop|restart|status|uninstall>

Options

`, exe)
	fs.PrintDefaults()
	fmt.Fprintf(w, `
Setup

  1. Run %[1]s onboard to create %%APPDATA%%\Dexgram\dexgram.toml.
  2. Or copy dexgram.example.toml to dexgram.toml and edit it manually.
  3. Put your Telegram bot token in [telegram].bot_token.
  4. Put your private bot chat id in [telegram].chat_id.
     Use chat_id = 0 temporarily to discover it from Dexgram logs.
  5. In @BotFather, open:
       Bot Settings -> Threads Settings
     Enable:
       Threaded Mode
  6. Run:
       %[1]s -check
  7. Start the daemon:
       %[1]s
     Or install the user-login scheduled task:
       %[1]s service install
       %[1]s service start

Config

  Default config: .\dexgram.toml if present, otherwise %%APPDATA%%\Dexgram\dexgram.toml
  Example config: .\dexgram.example.toml
  Service config: %%APPDATA%%\Dexgram\dexgram.toml

  Important [codex] values:
    cwd             legacy fallback for broken/stored mappings; normal
                    one-off chats prepare a dated projectless workspace under
                    %%USERPROFILE%%\Documents\Codex and register the Codex
                    thread for Desktop's projectless chat list
    cli_path        optional absolute path to codex.exe
    approval_policy "never" or "on-request"
    sandbox         "danger-full-access", "workspace-write", or "read-only"

Telegram Commands

  /new [title]
      Create a new Telegram topic for a one-off Codex chat.

  /new project query: title
      Create a topic pre-bound to the matched Codex project.

  /project <project name>
      Fuzzy-select the Codex project before the first real prompt in a topic.
      If the match is ambiguous, Dexgram sends inline selection buttons.
      Once a Codex thread exists, the topic cannot be moved to another project.

  /status
      Show this topic's Dexgram mapping, project/cwd, and active turn state.

  /sync
      Manually mirror completed Codex turns that are not yet marked as synced
      for this Telegram topic.

  /steer <message>
      Steer the currently active Codex turn.

  /goal <objective>
      Set the native Codex goal for this Telegram topic.

  /stop
  /cancel
      Interrupt the currently active Codex turn.

Runtime Behavior

  On the first prompt in a Telegram topic, Dexgram starts a Codex thread and
  saves the mapping by Telegram chat_id and message_thread_id. Later messages
  in that topic use the stored Codex thread id. In a topic with no /project,
  the first prompt creates a Codex one-off workspace similar to Desktop's
  "Don't work in a project" flow:

    %%USERPROFILE%%\Documents\Codex\YYYY-MM-DD\chat-title

  If a turn is already active, Dexgram submits the message to Codex anyway and
  lets Codex queue it natively. Dexgram replies to the queued Telegram message
  with buttons:

    Steer          merge that queued input into the active turn
    Delete queued  cancel that queued Codex turn

  Each Codex turn is presented as:

    1. Initial assistant/plan message
    2. One live run-log message for commands, tools, edits, searches, media
    3. Final assistant answer

  Telegram photos/image documents are sent to Codex as localImage inputs.
  Generic documents are downloaded, mentioned, and included by absolute path.
  Attachment-only messages are staged and attached to the next text prompt.
  Codex-created files are uploaded back to Telegram when detected.

Service Mode

  Dexgram service mode is a Windows Task Scheduler user-login task. It is not a
  Windows Service, so it runs as the signed-in user and can talk to the same
  Codex/Desktop context.

  Fixed service paths:
    Binary: %%LOCALAPPDATA%%\Dexgram\dexgram.exe
    Config: %%APPDATA%%\Dexgram\dexgram.toml
    Logs:   %%APPDATA%%\Dexgram\dexgram.log
    State:  %%APPDATA%%\Dexgram\dexgram.db

  Commands:
    %[1]s service install
    %[1]s service start
    %[1]s service stop
    %[1]s service restart
    %[1]s service status
    %[1]s service uninstall

  The future install command can place the binary at the fixed LocalAppData
  path. The scheduled task runs the binary directly with -config and -log.

Storage

  Local state: %[2]s
  Codex history remains in Codex's own session store under ~/.codex/sessions.

Examples

  Validate setup:
    %[1]s -check

  Run with explicit config:
    %[1]s -config .\dexgram.toml

  Discover chat id:
    Set chat_id = 0 in the config, run %[1]s, send the bot a DM,
    then copy the logged chat_id back into the config.

Notes

  Dexgram does not inject into the live Codex Desktop process. It talks to
  Codex through app-server and relies on Codex's persisted session history so
  Desktop can resume the same threads.

`, exe, statePath)
}
