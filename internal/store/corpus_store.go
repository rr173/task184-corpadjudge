package store

import (
	"database/sql"
	"errors"
	"fmt"

	"task184-corpadjudge/internal/model"
)

// CorpusStore 持久化语料片段，指纹唯一约束保证不重复入库。
type CorpusStore struct{ db *DB }

// NewCorpusStore 创建片段存储。
func NewCorpusStore(db *DB) *CorpusStore { return &CorpusStore{db: db} }

// CreateSpan 插入片段；指纹冲突返回 model.ErrDuplicate。
func (s *CorpusStore) CreateSpan(sp *model.CorpusSpan) (int64, error) {
	res, err := s.db.sql.Exec(`
		INSERT INTO corpus_spans(doc_id, start_offset, end_offset, text, fingerprint, layer, status, created_at, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sp.DocID, sp.StartOffset, sp.EndOffset, sp.Text, sp.Fingerprint, string(sp.Layer), sp.Status, sp.CreatedAt, sp.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, model.ErrDuplicate
		}
		return 0, fmt.Errorf("insert span: %w", err)
	}
	return res.LastInsertId()
}

// GetSpan 按 ID 读取片段。
func (s *CorpusStore) GetSpan(id int64) (*model.CorpusSpan, error) {
	var sp model.CorpusSpan
	var frozen sql.NullString
	err := s.db.sql.QueryRow(`
		SELECT id, doc_id, start_offset, end_offset, text, fingerprint, layer, status, created_at, updated_at, frozen_at
		FROM corpus_spans WHERE id = ?`, id).
		Scan(&sp.ID, &sp.DocID, &sp.StartOffset, &sp.EndOffset, &sp.Text, &sp.Fingerprint, &sp.Layer,
			&sp.Status, &sp.CreatedAt, &sp.UpdatedAt, &frozen)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if frozen.Valid {
		sp.FrozenAt = frozen.String
	}
	return &sp, nil
}

// GetSpanByFingerprint 按指纹读取片段（用于幂等）。
func (s *CorpusStore) GetSpanByFingerprint(fp string) (*model.CorpusSpan, error) {
	var sp model.CorpusSpan
	var frozen sql.NullString
	err := s.db.sql.QueryRow(`
		SELECT id, doc_id, start_offset, end_offset, text, fingerprint, layer, status, created_at, updated_at, frozen_at
		FROM corpus_spans WHERE fingerprint = ?`, fp).
		Scan(&sp.ID, &sp.DocID, &sp.StartOffset, &sp.EndOffset, &sp.Text, &sp.Fingerprint, &sp.Layer,
			&sp.Status, &sp.CreatedAt, &sp.UpdatedAt, &frozen)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if frozen.Valid {
		sp.FrozenAt = frozen.String
	}
	return &sp, nil
}

// ListSpans 分页列出片段，可按状态过滤。
func (s *CorpusStore) ListSpans(status string, limit, offset int) ([]model.CorpusSpan, error) {
	q := `SELECT id, doc_id, start_offset, end_offset, text, fingerprint, layer, status, created_at, updated_at, frozen_at
	      FROM corpus_spans`
	args := []any{}
	if status != "" {
		q += ` WHERE status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := s.db.sql.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.CorpusSpan{}
	for rows.Next() {
		var sp model.CorpusSpan
		var frozen sql.NullString
		if err := rows.Scan(&sp.ID, &sp.DocID, &sp.StartOffset, &sp.EndOffset, &sp.Text, &sp.Fingerprint, &sp.Layer,
			&sp.Status, &sp.CreatedAt, &sp.UpdatedAt, &frozen); err != nil {
			return nil, err
		}
		if frozen.Valid {
			sp.FrozenAt = frozen.String
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

// UpdateSpanStatus 更新片段状态与时间戳。
func (s *CorpusStore) UpdateSpanStatus(id int64, status, ts string) error {
	res, err := s.db.sql.Exec(`UPDATE corpus_spans SET status = ?, updated_at = ? WHERE id = ?`, status, ts, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrNotFound
	}
	return nil
}

// FreezeSpan 冻结片段（绑定基准时间）。
func (s *CorpusStore) FreezeSpan(id int64, ts string) error {
	res, err := s.db.sql.Exec(`UPDATE corpus_spans SET status = ?, frozen_at = ?, updated_at = ? WHERE id = ?`,
		model.SpanFrozen, ts, ts, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	return err != nil && (contains(err.Error(), "UNIQUE constraint failed") || contains(err.Error(), "constraint failed"))
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
