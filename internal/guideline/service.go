// Package guideline 管理准则版本与条款的生命周期。
//
// 状态机：draft → published → ambiguous → revoked。
// 已废止（revoked）版本不可再发布；条款引用只允许指向已发布或存在歧义的版本。
package guideline

import (
	"fmt"
	"strings"

	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/store"
)

// Service 提供准则版本与条款的业务操作。
type Service struct {
	store *store.GuidelineStore
}

// NewService 创建准则服务。
func NewService(s *store.GuidelineStore) *Service { return &Service{store: s} }

// CreateVersion 创建草拟准则版本；同名自动递增版本号。
func (s *Service) CreateVersion(name, description, note string) (*model.GuidelineVersion, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: name required", model.ErrInvalidInput)
	}
	no, err := s.store.NextVersionNo(name)
	if err != nil {
		return nil, err
	}
	v := &model.GuidelineVersion{
		Name:         name,
		Description:  description,
		Status:       model.GuidelineDraft,
		VersionNo:    no,
		CreatedAt:    store.Now(),
		RevisionNote: note,
	}
	id, err := s.store.CreateVersion(v)
	if err != nil {
		return nil, err
	}
	v.ID = id
	return v, nil
}

// Publish 将草拟版本发布（draft→published）。
func (s *Service) Publish(id int64) (*model.GuidelineVersion, error) {
	v, err := s.store.GetVersion(id)
	if err != nil {
		return nil, err
	}
	switch v.Status {
	case model.GuidelineDraft:
	case model.GuidelineAmbiguous:
		// 澄清歧义后可重新发布为已发布。
	default:
		return nil, fmt.Errorf("%w: cannot publish version in %q", model.ErrInvalidState, v.Status)
	}
	ts := store.Now()
	if err := s.store.UpdateVersionStatus(id, model.GuidelinePublished, "published", ts); err != nil {
		return nil, err
	}
	v.Status = model.GuidelinePublished
	v.PublishedAt = ts
	return v, nil
}

// MarkAmbiguous 标记已发布版本存在歧义（published→ambiguous）。
func (s *Service) MarkAmbiguous(id int64, note string) (*model.GuidelineVersion, error) {
	v, err := s.store.GetVersion(id)
	if err != nil {
		return nil, err
	}
	if v.Status != model.GuidelinePublished {
		return nil, fmt.Errorf("%w: only published version can be marked ambiguous, got %q", model.ErrInvalidState, v.Status)
	}
	ts := store.Now()
	if err := s.store.UpdateVersionStatus(id, model.GuidelineAmbiguous, "ambiguous", ts); err != nil {
		return nil, err
	}
	v.Status = model.GuidelineAmbiguous
	v.AmbiguousAt = ts
	v.RevisionNote = note
	return v, nil
}

// Revoke 废止版本（published/ambiguous→revoked）。
func (s *Service) Revoke(id int64, note string) (*model.GuidelineVersion, error) {
	v, err := s.store.GetVersion(id)
	if err != nil {
		return nil, err
	}
	switch v.Status {
	case model.GuidelinePublished, model.GuidelineAmbiguous, model.GuidelineDraft:
	default:
		return nil, fmt.Errorf("%w: already revoked", model.ErrInvalidState)
	}
	ts := store.Now()
	if err := s.store.UpdateVersionStatus(id, model.GuidelineRevoked, "revoked", ts); err != nil {
		return nil, err
	}
	v.Status = model.GuidelineRevoked
	v.RevokedAt = ts
	v.RevisionNote = note
	return v, nil
}

// AddClause 向草拟版本添加条款。
func (s *Service) AddClause(versionID int64, clauseNo string, layer model.Layer, title, body, scope string) (*model.GuidelineClause, error) {
	v, err := s.store.GetVersion(versionID)
	if err != nil {
		return nil, err
	}
	if v.Status != model.GuidelineDraft {
		return nil, fmt.Errorf("%w: clauses only editable in draft, got %q", model.ErrInvalidState, v.Status)
	}
	if !model.ValidLayer(layer) {
		return nil, fmt.Errorf("%w: unsupported layer %q", model.ErrInvalidInput, layer)
	}
	clauseNo = strings.TrimSpace(clauseNo)
	if clauseNo == "" || strings.TrimSpace(title) == "" {
		return nil, fmt.Errorf("%w: clause_no and title required", model.ErrInvalidInput)
	}
	c := &model.GuidelineClause{
		VersionID: versionID,
		ClauseNo:  clauseNo,
		Layer:     layer,
		Title:     strings.TrimSpace(title),
		Body:      body,
		Scope:     scope,
		Active:    true,
		CreatedAt: store.Now(),
	}
	id, err := s.store.CreateClause(c)
	if err != nil {
		return nil, err
	}
	c.ID = id
	return c, nil
}

// UpdateClause 更新草拟版本中的条款正文。
func (s *Service) UpdateClause(clauseID int64, title, body, scope string) (*model.GuidelineClause, error) {
	c, err := s.store.GetClause(clauseID)
	if err != nil {
		return nil, err
	}
	v, err := s.store.GetVersion(c.VersionID)
	if err != nil {
		return nil, err
	}
	if v.Status != model.GuidelineDraft {
		return nil, fmt.Errorf("%w: clauses only editable in draft, got %q", model.ErrInvalidState, v.Status)
	}
	if strings.TrimSpace(title) == "" {
		return nil, fmt.Errorf("%w: title required", model.ErrInvalidInput)
	}
	if err := s.store.UpdateClauseBody(clauseID, title, body, scope); err != nil {
		return nil, err
	}
	c.Title, c.Body, c.Scope = title, body, scope
	return c, nil
}

// Get 按 ID 读取准则版本。
func (s *Service) Get(id int64) (*model.GuidelineVersion, error) { return s.store.GetVersion(id) }

// ListAll 列出全部准则版本（新到旧）。
func (s *Service) ListAll() ([]model.GuidelineVersion, error) { return s.store.ListVersions() }

// ListClauses 列出某版本全部条款。
func (s *Service) ListClauses(versionID int64) ([]model.GuidelineClause, error) {
	// 影响分析按条款号读取版本快照。
	return s.store.ListClauses(versionID)
}

// ActiveVersion 返回指定版本 ID 的活动状态检查结果。
func (s *Service) ActiveVersion(id int64) (*model.GuidelineVersion, error) {
	v, err := s.store.GetVersion(id)
	if err != nil {
		return nil, err
	}
	switch v.Status {
	case model.GuidelinePublished, model.GuidelineAmbiguous:
		return v, nil
	}
	return nil, fmt.Errorf("%w: version %d status %q", model.ErrStaleGuideline, id, v.Status)
}
