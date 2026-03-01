package persistence

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/thwoodle/open-pilot/internal/core/session"
	"github.com/thwoodle/open-pilot/internal/domain"
	_ "modernc.org/sqlite"
)

const sqliteDriver = "sqlite"

type SQLitePersister struct {
	db   *sql.DB
	path string
}

func DefaultDBPath() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(cfgDir, "open-pilot", "sessions.db"), nil
}

func NewSQLitePersister(path string) (*SQLitePersister, error) {
	if path == "" {
		var err error
		path, err = DefaultDBPath()
		if err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir for session db: %w", err)
	}

	p, err := open(path)
	if err == nil {
		return p, nil
	}

	backupPath := path + ".corrupt." + time.Now().Format("20060102T150405")
	_ = os.Rename(path, backupPath)
	p2, err2 := open(path)
	if err2 != nil {
		return nil, fmt.Errorf("open sqlite persistence after recovery: %w (original: %v)", err2, err)
	}
	return p2, nil
}

func open(path string) (*SQLitePersister, error) {
	db, err := sql.Open(sqliteDriver, path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}
	p := &SQLitePersister{db: db, path: path}
	if err := p.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return p, nil
}

func (p *SQLitePersister) initSchema() error {
	stmts := []string{
		`PRAGMA foreign_keys = ON;`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			provider_id TEXT NOT NULL DEFAULT '',
			codex_thread_id TEXT NOT NULL DEFAULT '',
			active_repo_id TEXT NOT NULL DEFAULT '',
			created_at_unix INTEGER NOT NULL,
			sort_order INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS repos (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			path TEXT NOT NULL,
			label TEXT NOT NULL,
			sort_order INTEGER NOT NULL,
			FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			timestamp_unix INTEGER NOT NULL,
			provider_id TEXT NOT NULL DEFAULT '',
			repo_id TEXT NOT NULL DEFAULT '',
			streaming INTEGER NOT NULL DEFAULT 0,
			sort_order INTEGER NOT NULL,
			FOREIGN KEY(session_id) REFERENCES sessions(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS app_state (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
	}
	for _, stmt := range stmts {
		if _, err := p.db.Exec(stmt); err != nil {
			return fmt.Errorf("init sqlite schema: %w", err)
		}
	}
	if err := p.ensureSessionColumn("codex_thread_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	return nil
}

func (p *SQLitePersister) ensureSessionColumn(name, ddl string) error {
	rows, err := p.db.Query(`PRAGMA table_info(sessions)`)
	if err != nil {
		return fmt.Errorf("inspect sessions schema: %w", err)
	}
	defer rows.Close()

	hasColumn := false
	for rows.Next() {
		var (
			cid       int
			colName   string
			colType   string
			notNull   int
			dfltValue any
			pk        int
		)
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("scan sessions schema: %w", err)
		}
		if colName == name {
			hasColumn = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate sessions schema: %w", err)
	}
	if hasColumn {
		return nil
	}

	stmt := fmt.Sprintf("ALTER TABLE sessions ADD COLUMN %s %s", name, ddl)
	if _, err := p.db.Exec(stmt); err != nil {
		return fmt.Errorf("migrate sessions schema (%s): %w", name, err)
	}
	return nil
}

func (p *SQLitePersister) Save(snapshot session.Snapshot) error {
	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	clear := []string{
		`DELETE FROM messages;`,
		`DELETE FROM repos;`,
		`DELETE FROM sessions;`,
	}
	for _, stmt := range clear {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("clear tables: %w", err)
		}
	}

	for i, s := range snapshot.Sessions {
		if _, err := tx.Exec(`INSERT INTO sessions(id, name, provider_id, codex_thread_id, active_repo_id, created_at_unix, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			s.ID, s.Name, s.ProviderID, s.CodexThreadID, s.ActiveRepoID, s.CreatedAt, i); err != nil {
			return fmt.Errorf("insert session: %w", err)
		}
		for j, repo := range s.Repos {
			if _, err := tx.Exec(`INSERT INTO repos(id, session_id, path, label, sort_order) VALUES (?, ?, ?, ?, ?)`,
				repo.ID, s.ID, repo.Path, repo.Label, j); err != nil {
				return fmt.Errorf("insert repo: %w", err)
			}
		}
		for k, msg := range s.Messages {
			streamingInt := 0
			if msg.Streaming {
				streamingInt = 1
			}
			if _, err := tx.Exec(`INSERT INTO messages(id, session_id, role, content, timestamp_unix, provider_id, repo_id, streaming, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				msg.ID, s.ID, msg.Role, msg.Content, msg.Timestamp, msg.ProviderID, msg.RepoID, streamingInt, k); err != nil {
				return fmt.Errorf("insert message: %w", err)
			}
		}
	}

	if _, err := tx.Exec(`INSERT INTO app_state(key, value) VALUES('next_id', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, strconv.Itoa(snapshot.NextID)); err != nil {
		return fmt.Errorf("upsert app state: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (p *SQLitePersister) Load() (session.Snapshot, error) {
	snap := session.Snapshot{Sessions: []session.SessionSnapshot{}, NextID: 1}

	nextIDStr := ""
	err := p.db.QueryRow(`SELECT value FROM app_state WHERE key='next_id'`).Scan(&nextIDStr)
	if err == nil {
		if n, convErr := strconv.Atoi(nextIDStr); convErr == nil && n > 0 {
			snap.NextID = n
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return snap, fmt.Errorf("load app state: %w", err)
	}

	sRows, err := p.db.Query(`SELECT id, name, provider_id, codex_thread_id, active_repo_id, created_at_unix FROM sessions ORDER BY sort_order ASC`)
	if err != nil {
		return snap, fmt.Errorf("load sessions: %w", err)
	}
	defer sRows.Close()

	byID := map[string]*session.SessionSnapshot{}
	order := make([]string, 0)
	for sRows.Next() {
		var s session.SessionSnapshot
		if err := sRows.Scan(&s.ID, &s.Name, &s.ProviderID, &s.CodexThreadID, &s.ActiveRepoID, &s.CreatedAt); err != nil {
			return snap, fmt.Errorf("scan session: %w", err)
		}
		s.Repos = []domain.RepoRef{}
		s.Messages = []session.MessageSnapshot{}
		byID[s.ID] = &s
		order = append(order, s.ID)
	}
	if err := sRows.Err(); err != nil {
		return snap, fmt.Errorf("iterate sessions: %w", err)
	}

	rRows, err := p.db.Query(`SELECT id, session_id, path, label FROM repos ORDER BY sort_order ASC`)
	if err != nil {
		return snap, fmt.Errorf("load repos: %w", err)
	}
	defer rRows.Close()
	for rRows.Next() {
		var id, sessionID, path, label string
		if err := rRows.Scan(&id, &sessionID, &path, &label); err != nil {
			return snap, fmt.Errorf("scan repo: %w", err)
		}
		if s := byID[sessionID]; s != nil {
			s.Repos = append(s.Repos, domain.RepoRef{ID: id, Path: path, Label: label})
		}
	}
	if err := rRows.Err(); err != nil {
		return snap, fmt.Errorf("iterate repos: %w", err)
	}

	mRows, err := p.db.Query(`SELECT id, session_id, role, content, timestamp_unix, provider_id, repo_id, streaming FROM messages ORDER BY sort_order ASC`)
	if err != nil {
		return snap, fmt.Errorf("load messages: %w", err)
	}
	defer mRows.Close()
	for mRows.Next() {
		var id, sessionID, role, content, providerID, repoID string
		var ts int64
		var streaming int
		if err := mRows.Scan(&id, &sessionID, &role, &content, &ts, &providerID, &repoID, &streaming); err != nil {
			return snap, fmt.Errorf("scan message: %w", err)
		}
		if s := byID[sessionID]; s != nil {
			s.Messages = append(s.Messages, session.MessageSnapshot{
				ID:         id,
				Role:       role,
				Content:    content,
				Timestamp:  ts,
				ProviderID: providerID,
				RepoID:     repoID,
				Streaming:  false,
			})
		}
	}
	if err := mRows.Err(); err != nil {
		return snap, fmt.Errorf("iterate messages: %w", err)
	}

	for _, id := range order {
		if s := byID[id]; s != nil {
			snap.Sessions = append(snap.Sessions, *s)
		}
	}
	return snap, nil
}
