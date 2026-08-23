// Package baseline 管理冻结的基准集快照与准则更新引发的重审任务。
//
// 基准快照一旦冻结即为不可变：即使准则更新，旧快照仍保留原决定。
// 准则更新时，影响分析会为新案例生成重审任务（rereview_tasks）。
package baseline

import (
	"encoding/json"
	"fmt"

	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/store"
)

// Service 提供基准集业务操作。
type Service struct {
	bstore *store.BaselineStore
	astore *store.AdjudicationStore
	cstore *store.CorpusStore
}

// NewService 创建基准服务。
func NewService(b *store.BaselineStore, a *store.AdjudicationStore, c *store.CorpusStore) *Service {
	return &Service{bstore: b, astore: a, cstore: c}
}

// CreateSnapshot 创建草稿基准快照（未冻结）。
func (s *Service) CreateSnapshot(name string, guidelineVerID int64) (*model.BaselineSnapshot, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: name required", model.ErrInvalidInput)
	}
	snap := &model.BaselineSnapshot{
		Name: name, GuidelineVerID: guidelineVerID, CreatedAt: store.Now(),
	}
	id, err := s.bstore.CreateSnapshot(snap)
	if err != nil {
		return nil, err
	}
	snap.ID = id
	return snap, nil
}

// Freeze 冻结快照：把全部已决定/已规则化且未被冻结的案例写入快照，绑定片段指纹。
func (s *Service) Freeze(snapshotID int64, caseIDs []int64) (*model.BaselineSnapshot, error) {
	snap, err := s.bstore.GetSnapshot(snapshotID)
	if err != nil {
		return nil, err
	}
	if snap.Frozen {
		return nil, fmt.Errorf("%w: snapshot %d already frozen", model.ErrFrozen, snapshotID)
	}
	ts := store.Now()
	count := 0
	for _, caseID := range caseIDs {
		c, err := s.astore.Get(caseID)
		if err != nil {
			return nil, err
		}
		if c.Status != model.CaseDecided && c.Status != model.CaseFormalized {
			continue
		}
		if c.BaselineID > 0 {
			continue // 已入其他快照，跳过
		}
		sp, err := s.cstore.GetSpan(c.SpanID)
		if err != nil {
			return nil, err
		}
		clauseID := int64(0)
		if len(c.ClauseIDs) > 0 {
			clauseID = c.ClauseIDs[0]
		}
		bc := &model.BaselineCase{
			SnapshotID: snapshotID, CaseID: c.ID, SpanID: c.SpanID,
			SpanFingerprint: sp.Fingerprint, DecidedLabel: c.DecidedLabel,
			ClauseID: clauseID, GuidelineVerID: c.GuidelineVerID, FrozenAt: ts,
		}
		if _, err := s.bstore.AddBaselineCase(bc); err != nil {
			if err == model.ErrDuplicate {
				continue
			}
			return nil, err
		}
		_ = s.astore.BindBaseline(c.ID, snapshotID)
		count++
	}
	if err := s.bstore.FreezeSnapshot(snapshotID, count, ts); err != nil {
		return nil, err
	}
	return s.bstore.GetSnapshot(snapshotID)
}

// Get 读取快照详情与案例列表。
func (s *Service) Get(id int64) (*model.BaselineSnapshot, []model.BaselineCase, error) {
	snap, err := s.bstore.GetSnapshot(id)
	if err != nil {
		return nil, nil, err
	}
	cases, err := s.bstore.ListBaselineCases(id)
	if err != nil {
		return nil, nil, err
	}
	return snap, cases, nil
}

// List 列出全部快照。
func (s *Service) List() ([]model.BaselineSnapshot, error) { return s.bstore.ListSnapshots() }

// Export 导出快照为 JSON（含案例、指纹、条款与版本）。
func (s *Service) Export(id int64) (map[string]any, error) {
	snap, cases, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"snapshot":            snap,
		"guideline_version_id": snap.GuidelineVerID,
		"frozen":              snap.Frozen,
		"cases":               cases,
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	_ = b
	return out, nil
}
