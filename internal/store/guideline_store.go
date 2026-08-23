package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"task184-corpadjudge/internal/model"
)

// GuidelineStore 持久化准则版本与条款。
type GuidelineStore struct{ db *DB }

// NewGuidelineStore 创建准则存储。
func NewGuidelineStore(db *DB) *GuidelineStore { return &GuidelineStore{db: db} }

// CreateVersion 插入新的准则版本（draft）。
func (s *GuidelineStore) CreateVersion(v *model.GuidelineVersion) (int64, error) {
	res, err := s.db.sql.Exec(`
		INSERT INTO guideline_versions(name, description, status, version_no, created_at, revision_note)
		VALUES(?, ?, ?, ?, ?, ?)`,
		v.Name, v.Description, model.GuidelineDraft, v.VersionNo, v.CreatedAt, v.RevisionNote)
	if err != nil {
		return 0, fmt.Errorf("insert guideline version: %w", err)
	}
	return res.LastInsertId()
}

// GetVersion 按 ID 读取准则版本；不存在返回 model.ErrNotFound。
func (s *GuidelineStore) GetVersion(id int64) (*model.GuidelineVersion, error) {
	var v model.GuidelineVersion
	var rev, pub, amb, revk sql.NullString
	err := s.db.sql.QueryRow(`
		SELECT id, name, description, status, version_no, created_at, published_at, ambiguous_at, revoked_at, revision_note
		FROM guideline_versions WHERE id = ?`, id).
		Scan(&v.ID, &v.Name, &v.Description, &v.Status, &v.VersionNo, &v.CreatedAt,
			&pub, &amb, &revk, &rev)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if rev.Valid {
		v.RevisionNote = rev.String
	}
	if pub.Valid {
		v.PublishedAt = pub.String
	}
	if amb.Valid {
		v.AmbiguousAt = amb.String
	}
	if revk.Valid {
		v.RevokedAt = revk.String
	}
	return &v, nil
}

// UpdateVersionStatus 更新准则版本状态与时间戳（状态机由业务层守卫）。
func (s *GuidelineStore) UpdateVersionStatus(id int64, status, field, ts string) error {
	col := "status"
	switch field {
	case "published":
		col = "published_at"
	case "ambiguous":
		col = "ambiguous_at"
	case "revoked":
		col = "revoked_at"
	}
	_, err := s.db.sql.Exec(fmt.Sprintf(
		"UPDATE guideline_versions SET status = ?, %s = ? WHERE id = ?", col), status, ts, id)
	return err
}

// ListVersions 列出全部准则版本（新到旧）。
func (s *GuidelineStore) ListVersions() ([]model.GuidelineVersion, error) {
	rows, err := s.db.sql.Query(`
		SELECT id, name, description, status, version_no, created_at, published_at, ambiguous_at, revoked_at, revision_note
		FROM guideline_versions ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.GuidelineVersion{}
	for rows.Next() {
		var v model.GuidelineVersion
		var rev, pub, amb, revk sql.NullString
		if err := rows.Scan(&v.ID, &v.Name, &v.Description, &v.Status, &v.VersionNo, &v.CreatedAt,
			&pub, &amb, &revk, &rev); err != nil {
			return nil, err
		}
		if rev.Valid {
			v.RevisionNote = rev.String
		}
		if pub.Valid {
			v.PublishedAt = pub.String
		}
		if amb.Valid {
			v.AmbiguousAt = amb.String
		}
		if revk.Valid {
			v.RevokedAt = revk.String
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// NextVersionNo 返回该名称下的下一个版本号。
func (s *GuidelineStore) NextVersionNo(name string) (int, error) {
	var n int
	err := s.db.sql.QueryRow(`SELECT COALESCE(MAX(version_no), 0) + 1 FROM guideline_versions WHERE name = ?`, name).Scan(&n)
	return n, err
}

// CreateClause 插入条款。
func (s *GuidelineStore) CreateClause(c *model.GuidelineClause) (int64, error) {
	res, err := s.db.sql.Exec(`
		INSERT INTO guideline_clauses(version_id, clause_no, layer, title, body, scope, active, created_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		c.VersionID, c.ClauseNo, string(c.Layer), c.Title, c.Body, c.Scope, 1, c.CreatedAt)
	if err != nil {
		return 0, fmt.Errorf("insert clause: %w", err)
	}
	return res.LastInsertId()
}

// GetClause 按 ID 读取条款。
func (s *GuidelineStore) GetClause(id int64) (*model.GuidelineClause, error) {
	var c model.GuidelineClause
	var active int
	err := s.db.sql.QueryRow(`
		SELECT id, version_id, clause_no, layer, title, body, scope, active, created_at
		FROM guideline_clauses WHERE id = ?`, id).
		Scan(&c.ID, &c.VersionID, &c.ClauseNo, &c.Layer, &c.Title, &c.Body, &c.Scope, &active, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.Active = active == 1
	return &c, nil
}

// ListClauses 列出某版本的全部条款。
func (s *GuidelineStore) ListClauses(versionID int64) ([]model.GuidelineClause, error) {
	// 保持条款号顺序，供版本差异分析稳定比较。
	rows, err := s.db.sql.Query(`
		SELECT id, version_id, clause_no, layer, title, body, scope, active, created_at
		FROM guideline_clauses WHERE version_id = ? ORDER BY clause_no`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.GuidelineClause{}
	for rows.Next() {
		var c model.GuidelineClause
		var active int
		if err := rows.Scan(&c.ID, &c.VersionID, &c.ClauseNo, &c.Layer, &c.Title, &c.Body, &c.Scope, &active, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.Active = active == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateClauseBody 更新条款正文（仅 draft 版本允许，由业务层守卫）。
func (s *GuidelineStore) UpdateClauseBody(id int64, title, body, scope string) error {
	res, err := s.db.sql.Exec(`UPDATE guideline_clauses SET title = ?, body = ?, scope = ? WHERE id = ?`, title, body, scope, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrNotFound
	}
	return nil
}

// CountActiveClauses 统计某版本活动条款数。
func (s *GuidelineStore) CountActiveClauses(versionID int64) (int, error) {
	var n int
	err := s.db.sql.QueryRow(`SELECT COUNT(*) FROM guideline_clauses WHERE version_id = ? AND active = 1`, versionID).Scan(&n)
	return n, err
}

// ClausesJSON 返回条款 ID 列表的 JSON 表示（供裁决案例存储）。
func ClausesJSON(ids []int64) string {
	b, _ := json.Marshal(ids)
	return string(b)
}
