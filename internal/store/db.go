// Package store 提供基于 SQLite 的持久化层。
//
// 采用纯 Go 驱动 modernc.org/sqlite（CGO_ENABLED=0 可离线构建），
// 所有写操作均通过事务提交，为裁决与冻结提供可重复的原子性边界。
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// DB 封装 SQLite 连接与迁移。
type DB struct {
	sql *sql.DB
}

// Open 打开（必要时创建）数据库文件并执行建表迁移。
func Open(path string) (*DB, error) {
	if path != ":memory:" {
		if dir := filepath.Dir(path); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("mkdir db dir: %w", err)
			}
		}
	}
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	db := &DB{sql: sqlDB}
	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return db, nil
}

// SQL 暴露底层 *sql.DB，供事务与查询使用。
func (d *DB) SQL() *sql.DB { return d.sql }

// Close 关闭数据库连接。
func (d *DB) Close() error { return d.sql.Close() }

// Now 返回统一的 UTC 时间戳，供各层写入。
func Now() string { return time.Now().UTC().Format(time.RFC3339) }

// migrate 执行幂等建表迁移。
func (d *DB) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS guideline_versions (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	name         TEXT NOT NULL,
	description  TEXT NOT NULL DEFAULT '',
	status       TEXT NOT NULL DEFAULT 'draft',
	version_no   INTEGER NOT NULL DEFAULT 1,
	created_at   TEXT NOT NULL,
	published_at TEXT,
	ambiguous_at TEXT,
	revoked_at   TEXT,
	revision_note TEXT
);

CREATE TABLE IF NOT EXISTS guideline_clauses (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	version_id INTEGER NOT NULL REFERENCES guideline_versions(id),
	clause_no  TEXT NOT NULL,
	layer      TEXT NOT NULL,
	title      TEXT NOT NULL DEFAULT '',
	body       TEXT NOT NULL DEFAULT '',
	scope      TEXT NOT NULL DEFAULT '',
	active     INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL,
	UNIQUE (version_id, clause_no)
);
CREATE INDEX IF NOT EXISTS idx_clauses_version ON guideline_clauses(version_id);

CREATE TABLE IF NOT EXISTS corpus_spans (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	doc_id      TEXT NOT NULL,
	start_offset INTEGER NOT NULL,
	end_offset  INTEGER NOT NULL,
	text        TEXT NOT NULL,
	fingerprint TEXT NOT NULL UNIQUE,
	layer       TEXT NOT NULL,
	status      TEXT NOT NULL DEFAULT 'pending',
	created_at  TEXT NOT NULL,
	updated_at  TEXT NOT NULL,
	frozen_at   TEXT
);
CREATE INDEX IF NOT EXISTS idx_spans_doc ON corpus_spans(doc_id, start_offset);

CREATE TABLE IF NOT EXISTS annotations (
	id                INTEGER PRIMARY KEY AUTOINCREMENT,
	span_id           INTEGER NOT NULL REFERENCES corpus_spans(id),
	annotator         TEXT NOT NULL,
	layer             TEXT NOT NULL,
	label             TEXT NOT NULL,
	start_offset      INTEGER NOT NULL,
	end_offset        INTEGER NOT NULL,
	guideline_version_id INTEGER NOT NULL,
	status            TEXT NOT NULL DEFAULT 'draft',
	vote_key          TEXT NOT NULL UNIQUE,
	submitted_at      TEXT,
	decided_at        TEXT
);
CREATE INDEX IF NOT EXISTS idx_annotations_span ON annotations(span_id, layer);
CREATE INDEX IF NOT EXISTS idx_annotations_status ON annotations(status);

CREATE TABLE IF NOT EXISTS disputes (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	span_id     INTEGER NOT NULL REFERENCES corpus_spans(id),
	layer       TEXT NOT NULL,
	status      TEXT NOT NULL DEFAULT 'open',
	label_count INTEGER NOT NULL DEFAULT 0,
	created_at  TEXT NOT NULL,
	resolved_at TEXT,
	UNIQUE (span_id, layer)
);

CREATE TABLE IF NOT EXISTS matrix_entries (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	dispute_id INTEGER NOT NULL REFERENCES disputes(id),
	label      TEXT NOT NULL,
	annotators TEXT NOT NULL DEFAULT '',
	clause_refs TEXT NOT NULL DEFAULT '',
	vote_count INTEGER NOT NULL DEFAULT 0,
	UNIQUE (dispute_id, label)
);

CREATE TABLE IF NOT EXISTS adjudication_cases (
	id                INTEGER PRIMARY KEY AUTOINCREMENT,
	dispute_id        INTEGER NOT NULL UNIQUE REFERENCES disputes(id),
	span_id           INTEGER NOT NULL REFERENCES corpus_spans(id),
	status            TEXT NOT NULL DEFAULT 'pending',
	decided_label     TEXT,
	rationale         TEXT,
	clause_ids        TEXT NOT NULL DEFAULT '[]',
	guideline_version_id INTEGER NOT NULL,
	baseline_id       INTEGER NOT NULL DEFAULT 0,
	created_at        TEXT NOT NULL,
	decided_at        TEXT,
	formalized_rule   TEXT
);
CREATE INDEX IF NOT EXISTS idx_cases_span ON adjudication_cases(span_id);
CREATE INDEX IF NOT EXISTS idx_cases_status ON adjudication_cases(status);

CREATE TABLE IF NOT EXISTS baseline_snapshots (
	id                INTEGER PRIMARY KEY AUTOINCREMENT,
	name              TEXT NOT NULL,
	guideline_version_id INTEGER NOT NULL,
	case_count        INTEGER NOT NULL DEFAULT 0,
	frozen            INTEGER NOT NULL DEFAULT 0,
	created_at        TEXT NOT NULL,
	frozen_at         TEXT
);

CREATE TABLE IF NOT EXISTS baseline_cases (
	id                INTEGER PRIMARY KEY AUTOINCREMENT,
	snapshot_id       INTEGER NOT NULL REFERENCES baseline_snapshots(id),
	case_id           INTEGER NOT NULL REFERENCES adjudication_cases(id),
	span_id           INTEGER NOT NULL,
	span_fingerprint  TEXT NOT NULL,
	decided_label     TEXT NOT NULL,
	clause_id         INTEGER NOT NULL DEFAULT 0,
	guideline_version_id INTEGER NOT NULL,
	frozen_at         TEXT NOT NULL,
	UNIQUE (snapshot_id, case_id)
);
CREATE INDEX IF NOT EXISTS idx_baseline_cases_snapshot ON baseline_cases(snapshot_id);

CREATE TABLE IF NOT EXISTS rereview_tasks (
	id                INTEGER PRIMARY KEY AUTOINCREMENT,
	case_id           INTEGER NOT NULL,
	baseline_id       INTEGER NOT NULL,
	from_version_id   INTEGER NOT NULL,
	to_version_id     INTEGER NOT NULL,
	reason            TEXT NOT NULL,
	status            TEXT NOT NULL DEFAULT 'open',
	created_at        TEXT NOT NULL,
	resolved_at       TEXT,
	resolution_note   TEXT
);
CREATE INDEX IF NOT EXISTS idx_rereview_status ON rereview_tasks(status);
`
	_, err := d.sql.Exec(schema)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}
