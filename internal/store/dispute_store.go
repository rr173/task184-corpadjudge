package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"task184-corpadjudge/internal/model"
)


// scanDispute 从查询结果扫描一行分歧，nullable resolved_at 安全处理。
func scanDispute(s rowScanner) (*model.Dispute, error) {
	var d model.Dispute
	var res sql.NullString
	if err := s.Scan(&d.ID, &d.SpanID, &d.Layer, &d.Status, &d.LabelCount, &d.CreatedAt, &res); err != nil {
		return nil, err
	}
	if res.Valid {
		d.ResolvedAt = res.String
	}
	return &d, nil
}

// DisputeStore 持久化分歧记录与分歧矩阵条目。
type DisputeStore struct{ db *DB }

// NewDisputeStore 创建分歧存储。
func NewDisputeStore(db *DB) *DisputeStore { return &DisputeStore{db: db} }

// Create 创建分歧；同一 (span, layer) 已存在时返回 model.ErrDuplicate。
func (s *DisputeStore) Create(d *model.Dispute) (int64, error) {
	res, err := s.db.sql.Exec(`
		INSERT INTO disputes(span_id, layer, status, label_count, created_at)
		VALUES(?, ?, ?, ?, ?)`,
		d.SpanID, string(d.Layer), "open", d.LabelCount, d.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, model.ErrDuplicate
		}
		return 0, fmt.Errorf("insert dispute: %w", err)
	}
	return res.LastInsertId()
}

// Get 按 ID 读取分歧。
func (s *DisputeStore) Get(id int64) (*model.Dispute, error) {
	d, err := scanDispute(s.db.sql.QueryRow(`
		SELECT id, span_id, layer, status, label_count, created_at, resolved_at
		FROM disputes WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	return d, err
}

// GetBySpanLayer 按 (span_id, layer) 读取分歧。
func (s *DisputeStore) GetBySpanLayer(spanID int64, layer string) (*model.Dispute, error) {
	d, err := scanDispute(s.db.sql.QueryRow(`
		SELECT id, span_id, layer, status, label_count, created_at, resolved_at
		FROM disputes WHERE span_id = ? AND layer = ?`, spanID, layer))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	return d, err
}

// List 列出分歧，可按 span 过滤。
func (s *DisputeStore) List(spanID int64, limit, offset int) ([]model.Dispute, error) {
	q := `SELECT id, span_id, layer, status, label_count, created_at, resolved_at FROM disputes`
	args := []any{}
	if spanID > 0 {
		q += ` WHERE span_id = ?`
		args = append(args, spanID)
	}
	q += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := s.db.sql.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Dispute{}
	for rows.Next() {
		d, err := scanDispute(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// UpsertMatrixEntry 写入一行分歧矩阵（label 维度）。
func (s *DisputeStore) UpsertMatrixEntry(e model.MatrixEntry, disputeID int64) error {
	anns, _ := json.Marshal(e.Annotators)
	refs, _ := json.Marshal(e.ClauseRefs)
	_, err := s.db.sql.Exec(`
		INSERT INTO matrix_entries(dispute_id, label, annotators, clause_refs, vote_count)
		VALUES(?, ?, ?, ?, ?)
		ON CONFLICT(dispute_id, label) DO UPDATE SET
			annotators = excluded.annotators,
			clause_refs = excluded.clause_refs,
			vote_count = excluded.vote_count`,
		disputeID, e.Label, string(anns), string(refs), e.VoteCount)
	return err
}

// ListMatrix 读取分歧矩阵全部条目。
func (s *DisputeStore) ListMatrix(disputeID int64) ([]model.MatrixEntry, error) {
	rows, err := s.db.sql.Query(`
		SELECT label, annotators, clause_refs, vote_count FROM matrix_entries WHERE dispute_id = ? ORDER BY vote_count DESC`,
		disputeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.MatrixEntry{}
	for rows.Next() {
		var e model.MatrixEntry
		var anns, refs string
		if err := rows.Scan(&e.Label, &anns, &refs, &e.VoteCount); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(anns), &e.Annotators)
		json.Unmarshal([]byte(refs), &e.ClauseRefs)
		out = append(out, e)
	}
	return out, rows.Err()
}

// Resolve 将分歧标记为已解决。
func (s *DisputeStore) Resolve(id int64, ts string) error {
	_, err := s.db.sql.Exec(`UPDATE disputes SET status = 'resolved', resolved_at = ? WHERE id = ?`, ts, id)
	return err
}

// ClauseRefsString 解析条款引用 JSON 列表字符串。
func ClauseRefsString(s string) []int64 {
	var out []int64
	json.Unmarshal([]byte(s), &out)
	return out
}

// JoinAnnotators 将标注员列表拼接为逗号分隔字符串。
func JoinAnnotators(as []string) string { return strings.Join(as, ",") }
