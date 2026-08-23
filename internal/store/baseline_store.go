package store

import (
	"database/sql"
	"errors"
	"fmt"

	"task184-corpadjudge/internal/model"
)


// scanSnapshot 从查询结果扫描一行基准快照，nullable frozen_at 安全处理。
func scanSnapshot(s rowScanner) (*model.BaselineSnapshot, error) {
	var b model.BaselineSnapshot
	var frozen int
	var frozenAt sql.NullString
	if err := s.Scan(&b.ID, &b.Name, &b.GuidelineVerID, &b.CaseCount, &frozen, &b.CreatedAt, &frozenAt); err != nil {
		return nil, err
	}
	b.Frozen = frozen == 1
	if frozenAt.Valid {
		b.FrozenAt = frozenAt.String
	}
	return &b, nil
}

// BaselineStore 持久化基准快照、快照案例与重审任务。
type BaselineStore struct{ db *DB }

// NewBaselineStore 创建基准存储。
func NewBaselineStore(db *DB) *BaselineStore { return &BaselineStore{db: db} }

// CreateSnapshot 创建基准快照（draft，未冻结）。
func (s *BaselineStore) CreateSnapshot(snap *model.BaselineSnapshot) (int64, error) {
	res, err := s.db.sql.Exec(`
		INSERT INTO baseline_snapshots(name, guideline_version_id, case_count, frozen, created_at)
		VALUES(?, ?, ?, 0, ?)`,
		snap.Name, snap.GuidelineVerID, snap.CaseCount, snap.CreatedAt)
	if err != nil {
		return 0, fmt.Errorf("insert snapshot: %w", err)
	}
	return res.LastInsertId()
}

// GetSnapshot 按 ID 读取快照。
func (s *BaselineStore) GetSnapshot(id int64) (*model.BaselineSnapshot, error) {
	b, err := scanSnapshot(s.db.sql.QueryRow(`
		SELECT id, name, guideline_version_id, case_count, frozen, created_at, frozen_at
		FROM baseline_snapshots WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	return b, err
}

// ListSnapshots 列出全部基准快照。
func (s *BaselineStore) ListSnapshots() ([]model.BaselineSnapshot, error) {
	rows, err := s.db.sql.Query(`
		SELECT id, name, guideline_version_id, case_count, frozen, created_at, frozen_at
		FROM baseline_snapshots ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.BaselineSnapshot{}
	for rows.Next() {
		b, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

// FreezeSnapshot 冻结快照。
func (s *BaselineStore) FreezeSnapshot(id int64, caseCount int, ts string) error {
	res, err := s.db.sql.Exec(`
		UPDATE baseline_snapshots SET frozen = 1, case_count = ?, frozen_at = ? WHERE id = ? AND frozen = 0`,
		caseCount, ts, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrInvalidState
	}
	return nil
}

// AddBaselineCase 将案例写入快照（不可变条目）。
func (s *BaselineStore) AddBaselineCase(bc *model.BaselineCase) (int64, error) {
	res, err := s.db.sql.Exec(`
		INSERT INTO baseline_cases(snapshot_id, case_id, span_id, span_fingerprint, decided_label, clause_id, guideline_version_id, frozen_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		bc.SnapshotID, bc.CaseID, bc.SpanID, bc.SpanFingerprint, bc.DecidedLabel, bc.ClauseID, bc.GuidelineVerID, bc.FrozenAt)
	if err != nil {
		return 0, fmt.Errorf("insert baseline case: %w", err)
	}
	return res.LastInsertId()
}

// ListBaselineCases 列出快照内全部案例条目。
func (s *BaselineStore) ListBaselineCases(snapshotID int64) ([]model.BaselineCase, error) {
	rows, err := s.db.sql.Query(`
		SELECT id, snapshot_id, case_id, span_id, span_fingerprint, decided_label, clause_id, guideline_version_id, frozen_at
		FROM baseline_cases WHERE snapshot_id = ? ORDER BY id`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.BaselineCase{}
	for rows.Next() {
		var bc model.BaselineCase
		if err := rows.Scan(&bc.ID, &bc.SnapshotID, &bc.CaseID, &bc.SpanID, &bc.SpanFingerprint,
			&bc.DecidedLabel, &bc.ClauseID, &bc.GuidelineVerID, &bc.FrozenAt); err != nil {
			return nil, err
		}
		out = append(out, bc)
	}
	return out, rows.Err()
}

// CreateRereview 创建重审任务。
func (s *BaselineStore) CreateRereview(t *model.RereviewTask) (int64, error) {
	res, err := s.db.sql.Exec(`
		INSERT INTO rereview_tasks(case_id, baseline_id, from_version_id, to_version_id, reason, status, created_at)
		VALUES(?, ?, ?, ?, ?, 'open', ?)`,
		t.CaseID, t.BaselineID, t.FromVersionID, t.ToVersionID, t.Reason, t.CreatedAt)
	if err != nil {
		return 0, fmt.Errorf("insert rereview: %w", err)
	}
	return res.LastInsertId()
}

// ListRereviews 列出重审任务，可按状态过滤。
func (s *BaselineStore) ListRereviews(status string, limit, offset int) ([]model.RereviewTask, error) {
	q := `SELECT id, case_id, baseline_id, from_version_id, to_version_id, reason, status, created_at, resolved_at, resolution_note
	      FROM rereview_tasks`
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
	out := []model.RereviewTask{}
	for rows.Next() {
		var t model.RereviewTask
		var note sql.NullString
		var resAt sql.NullString
		if err := rows.Scan(&t.ID, &t.CaseID, &t.BaselineID, &t.FromVersionID, &t.ToVersionID,
			&t.Reason, &t.Status, &t.CreatedAt, &resAt, &note); err != nil {
			return nil, err
		}
		if resAt.Valid {
			t.ResolvedAt = resAt.String
		}
		if note.Valid {
			t.ResolutionNote = note.String
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ResolveRereview 解决重审任务。
func (s *BaselineStore) ResolveRereview(id int64, note, ts string) error {
	res, err := s.db.sql.Exec(`
		UPDATE rereview_tasks SET status = 'resolved', resolution_note = ?, resolved_at = ? WHERE id = ? AND status = 'open'`,
		note, ts, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrInvalidState
	}
	return nil
}

// CountRereviewsOpen 统计未解决重审任务数。
func (s *BaselineStore) CountRereviewsOpen() (int, error) {
	var n int
	err := s.db.sql.QueryRow(`SELECT COUNT(*) FROM rereview_tasks WHERE status = 'open'`).Scan(&n)
	return n, err
}
