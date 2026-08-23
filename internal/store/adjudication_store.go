package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"task184-corpadjudge/internal/model"
)

// AdjudicationStore 持久化裁决案例。
type AdjudicationStore struct{ db *DB }

// NewAdjudicationStore 创建裁决存储。
func NewAdjudicationStore(db *DB) *AdjudicationStore { return &AdjudicationStore{db: db} }

// Create 创建裁决案例（pending）。
func (s *AdjudicationStore) Create(c *model.AdjudicationCase) (int64, error) {
	// 案例持久化只负责保存传入的条款列表。
	clauses, _ := json.Marshal(c.ClauseIDs)
	res, err := s.db.sql.Exec(`
		INSERT INTO adjudication_cases(dispute_id, span_id, status, clause_ids, guideline_version_id, created_at)
		VALUES(?, ?, ?, ?, ?, ?)`,
		c.DisputeID, c.SpanID, model.CasePending, string(clauses), c.GuidelineVerID, c.CreatedAt)
	if err != nil {
		return 0, fmt.Errorf("insert case: %w", err)
	}
	return res.LastInsertId()
}

// scanCase 从查询结果扫描一行裁决案例，nullable 列安全处理。
func scanCase(s rowScanner) (*model.AdjudicationCase, error) {
	var c model.AdjudicationCase
	var clauses string
	var label, rationale, decidedAt, rule sql.NullString
	if err := s.Scan(&c.ID, &c.DisputeID, &c.SpanID, &c.Status, &label, &rationale, &clauses,
		&c.GuidelineVerID, &c.BaselineID, &c.CreatedAt, &decidedAt, &rule); err != nil {
		return nil, err
	}
	if label.Valid {
		c.DecidedLabel = label.String
	}
	if rationale.Valid {
		c.Rationale = rationale.String
	}
	if decidedAt.Valid {
		c.DecidedAt = decidedAt.String
	}
	if rule.Valid {
		c.FormalizedRule = rule.String
	}
	json.Unmarshal([]byte(clauses), &c.ClauseIDs)
	return &c, nil
}

// Get 按 ID 读取案例。
func (s *AdjudicationStore) Get(id int64) (*model.AdjudicationCase, error) {
	c, err := scanCase(s.db.sql.QueryRow(`
		SELECT id, dispute_id, span_id, status, decided_label, rationale, clause_ids,
			guideline_version_id, baseline_id, created_at, decided_at, formalized_rule
		FROM adjudication_cases WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	return c, err
}

// GetByDispute 按分歧 ID 读取案例（唯一）。
func (s *AdjudicationStore) GetByDispute(disputeID int64) (*model.AdjudicationCase, error) {
	c, err := scanCase(s.db.sql.QueryRow(`
		SELECT id, dispute_id, span_id, status, decided_label, rationale, clause_ids,
			guideline_version_id, baseline_id, created_at, decided_at, formalized_rule
		FROM adjudication_cases WHERE dispute_id = ?`, disputeID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	return c, err
}

// List 分页列出案例，可按状态过滤。
func (s *AdjudicationStore) List(status string, limit, offset int) ([]model.AdjudicationCase, error) {
	q := `SELECT id, dispute_id, span_id, status, decided_label, rationale, clause_ids,
		guideline_version_id, baseline_id, created_at, decided_at, formalized_rule
		FROM adjudication_cases`
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
	out := []model.AdjudicationCase{}
	for rows.Next() {
		c, err := scanCase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// ListDecided 列出已决定或已规则化（可入基准）的案例。
func (s *AdjudicationStore) ListDecided(limit, offset int) ([]model.AdjudicationCase, error) {
	rows, err := s.db.sql.Query(`
		SELECT id, dispute_id, span_id, status, decided_label, rationale, clause_ids,
			guideline_version_id, baseline_id, created_at, decided_at, formalized_rule
		FROM adjudication_cases WHERE status IN (?, ?) ORDER BY id DESC LIMIT ? OFFSET ?`,
		model.CaseDecided, model.CaseFormalized, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.AdjudicationCase{}
	for rows.Next() {
		c, err := scanCase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// Decide 记录裁决决定（pending→decided）。
func (s *AdjudicationStore) Decide(id int64, label, rationale string, clauseIDs []int64, ts string) error {
	clauses, _ := json.Marshal(clauseIDs)
	res, err := s.db.sql.Exec(`
		UPDATE adjudication_cases SET status = ?, decided_label = ?, rationale = ?, clause_ids = ?, decided_at = ?
		WHERE id = ? AND status = ?`,
		model.CaseDecided, label, rationale, string(clauses), ts, id, model.CasePending)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrInvalidState
	}
	return nil
}

// Formalize 将案例规则化（decided→formalized）。
func (s *AdjudicationStore) Formalize(id int64, rule string) error {
	res, err := s.db.sql.Exec(`
		UPDATE adjudication_cases SET status = ?, formalized_rule = ? WHERE id = ? AND status = ?`,
		model.CaseFormalized, rule, id, model.CaseDecided)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrInvalidState
	}
	return nil
}

// BindBaseline 将案例绑定到基准快照。
func (s *AdjudicationStore) BindBaseline(id, baselineID int64) error {
	_, err := s.db.sql.Exec(`UPDATE adjudication_cases SET baseline_id = ? WHERE id = ?`, baselineID, id)
	return err
}

// MarkSuperseded 标记案例为已替代。
func (s *AdjudicationStore) MarkSuperseded(id int64) error {
	_, err := s.db.sql.Exec(`UPDATE adjudication_cases SET status = ? WHERE id = ?`, model.CaseSuperseded, id)
	return err
}

// ListByVersion 列出引用某准则版本的案例（用于影响分析）。
func (s *AdjudicationStore) ListByVersion(versionID int64) ([]model.AdjudicationCase, error) {
	rows, err := s.db.sql.Query(`
		SELECT id, dispute_id, span_id, status, decided_label, rationale, clause_ids,
			guideline_version_id, baseline_id, created_at, decided_at, formalized_rule
		FROM adjudication_cases WHERE guideline_version_id = ? AND status IN (?, ?)`,
		versionID, model.CaseDecided, model.CaseFormalized)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.AdjudicationCase{}
	for rows.Next() {
		c, err := scanCase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}
