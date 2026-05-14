package state

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	path string
	db   *sql.DB
}

type Conversation struct {
	ChatID                int64
	MessageThreadID       int
	CodexThreadID         string
	ProjectName           string
	CWD                   string
	Projectless           bool
	TopicTitle            string
	TopicNamed            bool
	SideChat              bool
	ParentChatID          int64
	ParentMessageThreadID int
	ParentCodexThreadID   string
	SideIndex             int
	LastSyncedTurnID      string
	UpdatedAt             string
}

type StagedAttachment struct {
	ID              int64
	ChatID          int64
	MessageThreadID int
	MessageID       int
	Path            string
	Kind            string
	Name            string
	CreatedAt       string
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "Dexgram", "dexgram.db"), nil
}

func Open(path string) (*Store, error) {
	if path == "" {
		var err error
		path, err = DefaultPath()
		if err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &Store{path: path, db: db}
	if err := s.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) init() error {
	_, err := s.db.Exec(`
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;

CREATE TABLE IF NOT EXISTS conversations (
  chat_id INTEGER NOT NULL,
  message_thread_id INTEGER NOT NULL,
  codex_thread_id TEXT NOT NULL DEFAULT '',
  project_name TEXT NOT NULL DEFAULT '',
  cwd TEXT NOT NULL DEFAULT '',
  projectless INTEGER NOT NULL DEFAULT 0,
  topic_title TEXT NOT NULL DEFAULT '',
  topic_named INTEGER NOT NULL DEFAULT 0,
  last_synced_turn_id TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL,
  PRIMARY KEY (chat_id, message_thread_id)
);

CREATE TABLE IF NOT EXISTS staged_attachments (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  chat_id INTEGER NOT NULL,
  message_thread_id INTEGER NOT NULL,
  message_id INTEGER NOT NULL,
  path TEXT NOT NULL,
  kind TEXT NOT NULL,
  name TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS telegram_pairing_codes (
  code TEXT PRIMARY KEY,
  chat_id INTEGER NOT NULL,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);`)
	if err != nil {
		return err
	}
	for _, column := range []struct {
		name string
		ddl  string
	}{
		{name: "side_chat", ddl: "side_chat INTEGER NOT NULL DEFAULT 0"},
		{name: "parent_chat_id", ddl: "parent_chat_id INTEGER NOT NULL DEFAULT 0"},
		{name: "parent_message_thread_id", ddl: "parent_message_thread_id INTEGER NOT NULL DEFAULT 0"},
		{name: "parent_codex_thread_id", ddl: "parent_codex_thread_id TEXT NOT NULL DEFAULT ''"},
		{name: "side_index", ddl: "side_index INTEGER NOT NULL DEFAULT 0"},
	} {
		if err := s.ensureColumn("conversations", column.name, column.ddl); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Get(chatID int64, messageThreadID int) (Conversation, bool, error) {
	row := s.db.QueryRow(`
SELECT chat_id, message_thread_id, codex_thread_id, project_name, cwd, projectless,
       topic_title, topic_named, side_chat, parent_chat_id, parent_message_thread_id,
       parent_codex_thread_id, side_index, last_synced_turn_id, updated_at
FROM conversations
WHERE chat_id = ? AND message_thread_id = ?`, chatID, messageThreadID)
	conv, err := scanConversation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Conversation{}, false, nil
	}
	if err != nil {
		return Conversation{}, false, err
	}
	return conv, true, nil
}

func (s *Store) Upsert(conv Conversation) error {
	conv.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
INSERT INTO conversations (
  chat_id, message_thread_id, codex_thread_id, project_name, cwd, projectless,
  topic_title, topic_named, side_chat, parent_chat_id, parent_message_thread_id,
  parent_codex_thread_id, side_index, last_synced_turn_id, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(chat_id, message_thread_id) DO UPDATE SET
  codex_thread_id = excluded.codex_thread_id,
  project_name = excluded.project_name,
  cwd = excluded.cwd,
  projectless = excluded.projectless,
  topic_title = excluded.topic_title,
  topic_named = excluded.topic_named,
  side_chat = excluded.side_chat,
  parent_chat_id = excluded.parent_chat_id,
  parent_message_thread_id = excluded.parent_message_thread_id,
  parent_codex_thread_id = excluded.parent_codex_thread_id,
  side_index = excluded.side_index,
  last_synced_turn_id = excluded.last_synced_turn_id,
  updated_at = excluded.updated_at`,
		conv.ChatID,
		conv.MessageThreadID,
		conv.CodexThreadID,
		conv.ProjectName,
		conv.CWD,
		boolInt(conv.Projectless),
		conv.TopicTitle,
		boolInt(conv.TopicNamed),
		boolInt(conv.SideChat),
		conv.ParentChatID,
		conv.ParentMessageThreadID,
		conv.ParentCodexThreadID,
		conv.SideIndex,
		conv.LastSyncedTurnID,
		conv.UpdatedAt,
	)
	return err
}

func (s *Store) NextSideIndex(parentChatID int64, parentMessageThreadID int) (int, error) {
	row := s.db.QueryRow(`
SELECT COALESCE(MAX(side_index), 0) + 1
FROM conversations
WHERE side_chat != 0 AND parent_chat_id = ? AND parent_message_thread_id = ?`,
		parentChatID,
		parentMessageThreadID,
	)
	var index int
	if err := row.Scan(&index); err != nil {
		return 0, err
	}
	return index, nil
}

func (s *Store) AddStagedAttachment(attachment StagedAttachment) error {
	attachment.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
INSERT INTO staged_attachments (
  chat_id, message_thread_id, message_id, path, kind, name, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		attachment.ChatID,
		attachment.MessageThreadID,
		attachment.MessageID,
		attachment.Path,
		attachment.Kind,
		attachment.Name,
		attachment.CreatedAt,
	)
	return err
}

func (s *Store) ListStagedAttachments(chatID int64, messageThreadID int) ([]StagedAttachment, error) {
	rows, err := s.db.Query(`
SELECT id, chat_id, message_thread_id, message_id, path, kind, name, created_at
FROM staged_attachments
WHERE chat_id = ? AND message_thread_id = ?
ORDER BY id`, chatID, messageThreadID)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()
	var out []StagedAttachment
	for rows.Next() {
		var attachment StagedAttachment
		if err := rows.Scan(
			&attachment.ID,
			&attachment.ChatID,
			&attachment.MessageThreadID,
			&attachment.MessageID,
			&attachment.Path,
			&attachment.Kind,
			&attachment.Name,
			&attachment.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, attachment)
	}
	return out, rows.Err()
}

func (s *Store) ClearStagedAttachments(chatID int64, messageThreadID int) error {
	_, err := s.db.Exec(`
DELETE FROM staged_attachments
WHERE chat_id = ? AND message_thread_id = ?`, chatID, messageThreadID)
	return err
}

func (s *Store) GetSetting(key string) (string, error) {
	row := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key)
	var value string
	if err := row.Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return value, nil
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`
INSERT INTO settings (key, value, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET
  value = excluded.value,
  updated_at = excluded.updated_at`,
		key,
		value,
		time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (s *Store) SaveTelegramPairingCode(code string, chatID int64, expiresAt time.Time) error {
	_, err := s.db.Exec(`
INSERT INTO telegram_pairing_codes (code, chat_id, expires_at, created_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(code) DO UPDATE SET
  chat_id = excluded.chat_id,
  expires_at = excluded.expires_at,
  created_at = excluded.created_at`,
		code,
		chatID,
		expiresAt.UTC().Format(time.RFC3339),
		time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (s *Store) ConsumeTelegramPairingCode(code string) (int64, bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, false, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec(`DELETE FROM telegram_pairing_codes WHERE expires_at <= ?`, now); err != nil {
		return 0, false, err
	}

	row := tx.QueryRow(`SELECT chat_id FROM telegram_pairing_codes WHERE code = ?`, code)
	var chatID int64
	if err := row.Scan(&chatID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if err := tx.Commit(); err != nil {
				return 0, false, err
			}
			committed = true
			return 0, false, nil
		}
		return 0, false, err
	}
	if _, err := tx.Exec(`DELETE FROM telegram_pairing_codes WHERE code = ?`, code); err != nil {
		return 0, false, err
	}
	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	committed = true
	return chatID, true, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanConversation(row scanner) (Conversation, error) {
	var conv Conversation
	var projectless int
	var topicNamed int
	var sideChat int
	if err := row.Scan(
		&conv.ChatID,
		&conv.MessageThreadID,
		&conv.CodexThreadID,
		&conv.ProjectName,
		&conv.CWD,
		&projectless,
		&conv.TopicTitle,
		&topicNamed,
		&sideChat,
		&conv.ParentChatID,
		&conv.ParentMessageThreadID,
		&conv.ParentCodexThreadID,
		&conv.SideIndex,
		&conv.LastSyncedTurnID,
		&conv.UpdatedAt,
	); err != nil {
		return Conversation{}, fmt.Errorf("scan conversation: %w", err)
	}
	conv.Projectless = projectless != 0
	conv.TopicNamed = topicNamed != 0
	conv.SideChat = sideChat != 0
	return conv, nil
}

func (s *Store) ensureColumn(table, name, ddl string) error {
	rows, err := s.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer func() {
		_ = rows.Close()
	}()
	for rows.Next() {
		var cid int
		var columnName string
		var columnType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&cid, &columnName, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if columnName == name {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec("ALTER TABLE " + table + " ADD COLUMN " + ddl)
	return err
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
