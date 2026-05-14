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
	_, _ = fmt.Fprintf(w, `Dexgram

  Telegram bridge for Codex Desktop / Codex app-server threads.
  It runs as a single Windows binary, listens to a private Telegram bot chat
  with threaded topics enabled, and maps each Telegram topic to one Codex
  thread.

Usage

  %[1]s [options]
  %[1]s -check
  %[1]s -config .\dexgram.toml
  %[1]s version
  %[1]s onboard
  %[1]s telegram chatid add <chat_id>
  %[1]s update
  %[1]s service <install|start|stop|restart|status|uninstall>

Options

`, exe)
	fs.PrintDefaults()
	_, _ = fmt.Fprintf(w, `
Setup

  1. Run %[1]s onboard to create %%APPDATA%%\Dexgram\dexgram.toml.
  2. Or copy dexgram.example.toml beside the binary, then run onboard.
  3. Put your Telegram bot token in [telegram].bot_token.
  4. Add one or more Telegram chat ids:
       %[1]s telegram chatid add <chat_id>
     When a chat is not in chat_ids, Dexgram replies with a ready-to-run add
     command. Codex prompts and slash commands stay disabled for unauthorized
     chats.
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

Update

  %[1]s update
      Check GitHub for a newer release. If one exists, run the installer in
      update mode, replace %%LOCALAPPDATA%%\Dexgram\dexgram.exe, and skip
      onboarding. After a successful update, start the Dexgram service.

Version

  %[1]s version
      Print the Dexgram version and exit.

Telegram Config

  %[1]s telegram chatid add <chat_id>
  %[1]s telegram chatid del <chat_id>
  %[1]s telegram chatid clear
  %[1]s tg id add <chat_id>
      Update [telegram].chat_ids in the Dexgram TOML config without opening the
      file by hand. add accepts positive private chat ids and negative group ids.
      del removes one id. clear removes all ids, so every incoming chat is
      unauthorized and receives only setup instructions. Add -config before
      the action to target a non-default config.
      Telegram slash commands are registered only for configured chats.

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

  /plan <message>
      Start a true Codex Plan Mode turn in this Telegram topic.

  /settings
      Show the model and reasoning effort used for Telegram-started Plan Mode.

  /model [model-id|auto]
      Choose the model for Telegram-started Plan Mode turns. Without an
      argument, opens an inline selection menu.

  /effort [auto|minimal|low|medium|high|xhigh]
      Choose the reasoning effort for Telegram-started Plan Mode turns.
      Without an argument, opens an inline selection menu.

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

  If a turn is already active, Dexgram keeps the message queued locally and
  submits it to Codex when earlier work finishes. Dexgram replies to the queued
  Telegram message with buttons:

    Steer          merge that queued input into the active turn
    Delete         delete that queued message before it is submitted to Codex

  Each Codex turn is presented as:

    1. Initial assistant/plan message
    2. One live run-log message for commands, tools, edits, searches, media
    3. Final assistant answer

  Live status and run-log messages are sent silently. Final answers, approval
  prompts, and user-input prompts use normal Telegram notifications.

  Telegram photos/image documents are sent to Codex as localImage inputs.
  Generic documents are downloaded, mentioned, and included by absolute path.
  Attachment-only messages are staged and attached to the next text prompt.
  Final-answer local file links are uploaded only when
  [telegram].upload_final_answer_files is true.

Service Mode

  Dexgram service mode prefers a current-user Windows Task Scheduler login
  task. If Task Scheduler denies access, Dexgram installs a per-user Startup
  folder fallback. It is not a Windows Service, so it runs as the signed-in user
  and can talk to the same Codex/Desktop context.

  Fixed service paths:
    Binary: %%LOCALAPPDATA%%\Dexgram\dexgram.exe
    Config: %%APPDATA%%\Dexgram\dexgram.toml
    Logs:   %%APPDATA%%\Dexgram\dexgram.log
    State:  %%APPDATA%%\Dexgram\dexgram.db

  The service log keeps the newest %[3]d lines.

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

  Add a chat id:
    Run %[1]s, then send the bot a DM from any unauthorized chat.
    Dexgram replies with the exact %[1]s telegram chatid add command to run.

Notes

  Dexgram does not inject into the live Codex Desktop process. It talks to
  Codex through app-server and relies on Codex's persisted session history so
  Desktop can resume the same threads.

`, exe, statePath, logFileMaxLines)
}
