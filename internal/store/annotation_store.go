package store

import (
	"database/sql"
	"errors"
	"fmt"

	"task184-corpadjudge/internal/model"
)

// rowScanner 抽象 *sql.Row 与 *sql.Rows 的 Scan。
type rowScanner interface {
	Scan(dest ...any) error
}

// scanAnnotation 从查询结果扫描一行标注，nullable 时间列安全处理。
func scanAnnotation(s rowScanner) (*model.Annotation, error) {
	var a model.Annotation
	var sub, dec sql.NullString
	if err := s.Scan(&a.ID, &a.SpanID, &a.Annotator, &a.Layer, &a.Label, &a.StartOffset, &a.EndOffset,
		&a.GuidelineVerID, &a.Status, &a.VoteKey, &sub, &dec); err != nil {
		return nil, err
	}
	if sub.Valid {
		a.SubmittedAt = sub.String
	}
	if dec.Valid {
		a.DecidedAt = dec.String
	}
	return &a, nil
}

// AnnotationStore 持久化标注，vote_key 唯一约束实现幂等去重。
type AnnotationStore struct{ db *DB }

// NewAnnotationStore 创建标注存储。
func NewAnnotationStore(db *DB) *AnnotationStore { return &AnnotationStore{db: db} }

// Create 插入标注（draft）；vote_key 冲突返回 model.ErrDuplicate。
func (s *AnnotationStore) Create(a *model.Annotation) (int64, error) {
	res, err := s.db.sql.Exec(`
		INSERT INTO annotations(span_id, annotator, layer, label, start_offset, end_offset,
			guideline_version_id, status, vote_key)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.SpanID, a.Annotator, string(a.Layer), a.Label, a.StartOffset, a.EndOffset,
		a.GuidelineVerID, a.Status, a.VoteKey)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, model.ErrDuplicate
		}
		return 0, fmt.Errorf("insert annotation: %w", err)
	}
	return res.LastInsertId()
}

// Get 按 ID 读取标注。
func (s *AnnotationStore) Get(id int64) (*model.Annotation, error) {
	a, err := scanAnnotation(s.db.sql.QueryRow(`
		SELECT id, span_id, annotator, layer, label, start_offset, end_offset,
			guideline_version_id, status, vote_key, submitted_at, decided_at
		FROM annotations WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	return a, err
}

// GetByVoteKey 按幂等键读取标注（用于重复投票时返回既有记录）。
func (s *AnnotationStore) GetByVoteKey(key string) (*model.Annotation, error) {
	a, err := scanAnnotation(s.db.sql.QueryRow(`
		SELECT id, span_id, annotator, layer, label, start_offset, end_offset,
			guideline_version_id, status, vote_key, submitted_at, decided_at
		FROM annotations WHERE vote_key = ?`, key))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	return a, err
}

// UpdateDraft 更新草稿标注的标签与边界。
func (s *AnnotationStore) UpdateDraft(id int64, label string, start, end int) error {
	// 仅 status=draft 的行可被更新；片段冻结关系由业务层读取。
	res, err := s.db.sql.Exec(`UPDATE annotations SET label = ?, start_offset = ?, end_offset = ? WHERE id = ? AND status = ?`,
		label, start, end, id, model.AnnDraft)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrInvalidState
	}
	return nil
}

// Submit 提交标注（draft→submitted）。
func (s *AnnotationStore) Submit(id int64, ts string) error {
	res, err := s.db.sql.Exec(`UPDATE annotations SET status = ?, submitted_at = ? WHERE id = ? AND status = ?`,
		model.AnnSubmitted, ts, id, model.AnnDraft)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrInvalidState
	}
	return nil
}

// ListBySpan 列出某片段某层全部标注。
func (s *AnnotationStore) ListBySpan(spanID int64, layer string) ([]model.Annotation, error) {
	rows, err := s.db.sql.Query(`
		SELECT id, span_id, annotator, layer, label, start_offset, end_offset,
			guideline_version_id, status, vote_key, submitted_at, decided_at
		FROM annotations WHERE span_id = ? AND (? = '' OR layer = ?) ORDER BY id`, spanID, layer, layer)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Annotation{}
	for rows.Next() {
		a, err := scanAnnotation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// ListSubmitted 列出某片段某层已提交标注。
func (s *AnnotationStore) ListSubmitted(spanID int64, layer string) ([]model.Annotation, error) {
	rows, err := s.db.sql.Query(`
		SELECT id, span_id, annotator, layer, label, start_offset, end_offset,
			guideline_version_id, status, vote_key, submitted_at, decided_at
		FROM annotations WHERE span_id = ? AND layer = ? AND status IN (?, ?, ?, ?) ORDER BY id`,
		spanID, layer, model.AnnSubmitted, model.AnnAccepted, model.AnnRejected, model.AnnRereview)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Annotation{}
	for rows.Next() {
		a, err := scanAnnotation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// SetDecision 更新标注为被采纳/被驳回，记录决定时间。
func (s *AnnotationStore) SetDecision(id int64, status, ts string) error {
	res, err := s.db.sql.Exec(`UPDATE annotations SET status = ?, decided_at = ? WHERE id = ? AND status = ?`,
		status, ts, id, model.AnnSubmitted)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrInvalidState
	}
	return nil
}

// MarkRereview 将已采纳标注标记为待重审（准则更新触发）。
func (s *AnnotationStore) MarkRereview(id int64, ts string) error {
	res, err := s.db.sql.Exec(`UPDATE annotations SET status = ?, decided_at = ? WHERE id = ? AND status IN (?, ?)`,
		model.AnnRereview, ts, id, model.AnnAccepted, model.AnnRereview)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrInvalidState
	}
	return nil
}

// CountByStatus 统计某状态标注数量。
func (s *AnnotationStore) CountByStatus(status string) (int, error) {
	var n int
	err := s.db.sql.QueryRow(`SELECT COUNT(*) FROM annotations WHERE status = ?`, status).Scan(&n)
	return n, err
}
