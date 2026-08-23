// Package dispute 计算并查询语料片段上的标注分歧。
//
// 分歧按 (span, layer) 唯一组织，矩阵条目记录每个标签的投票人、票数与条款引用。
package dispute

import (
	"fmt"

	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/store"
)

// Service 提供分歧查询与矩阵计算。
type Service struct {
	store *store.DisputeStore
	anns  *store.AnnotationStore
}

// NewService 创建分歧服务。
func NewService(s *store.DisputeStore, a *store.AnnotationStore) *Service {
	return &Service{store: s, anns: a}
}

// List 分页列出分歧，可按 span 过滤。
func (s *Service) List(spanID int64, limit, offset int) ([]model.Dispute, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return s.store.List(spanID, limit, offset)
}

// Get 读取分歧详情。
func (s *Service) Get(id int64) (*model.Dispute, error) { return s.store.Get(id) }

// Matrix 计算并返回分歧矩阵。
func (s *Service) Matrix(disputeID int64) ([]model.MatrixEntry, error) {
	// 矩阵条目按标签聚合，保留每个标签下的投票人。
	d, err := s.store.Get(disputeID)
	if err != nil {
		return nil, err
	}
	// 重建矩阵：以当前已提交标注为准。
	anns, err := s.anns.ListSubmitted(d.SpanID, string(d.Layer))
	if err != nil {
		return nil, err
	}
	byLabel := map[string]*model.MatrixEntry{}
	order := []string{}
	for _, a := range anns {
		e, ok := byLabel[a.Label]
		if !ok {
			e = &model.MatrixEntry{Label: a.Label, Annotators: []string{}, ClauseRefs: []int64{}}
			byLabel[a.Label] = e
			order = append(order, a.Label)
		}
		e.Annotators = appendUnique(e.Annotators, a.Annotator)
		e.VoteCount = len(e.Annotators) // 按标注员去重计票，避免跨版本/重审重复行造成重复计数
	}
	out := make([]model.MatrixEntry, 0, len(order))
	for _, label := range order {
		e := *byLabel[label]
		out = append(out, e)
		_ = s.store.UpsertMatrixEntry(e, disputeID)
	}
	return out, nil
}

// ReferenceClauses 为矩阵中某标签条目补充条款引用（裁决者标注依据）。
func (s *Service) ReferenceClauses(disputeID int64, label string, clauseIDs []int64) error {
	entries, err := s.store.ListMatrix(disputeID)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Label == label {
			e.ClauseRefs = clauseIDs
			return s.store.UpsertMatrixEntry(e, disputeID)
		}
	}
	return fmt.Errorf("%w: label %q not in dispute matrix", model.ErrInvalidInput, label)
}

// Resolve 将分歧标记为已解决（裁决完成后调用）。
func (s *Service) Resolve(id int64) error { return s.store.Resolve(id, store.Now()) }

func appendUnique(list []string, v string) []string {
	for _, e := range list {
		if e == v {
			return list
		}
	}
	return append(list, v)
}
