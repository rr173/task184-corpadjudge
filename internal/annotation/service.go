// Package annotation 管理标注员对语料片段的标签投票。
//
// 标注状态机：draft → submitted → accepted/rejected → rereview（准则更新触发）。
// 幂等键 vote_key = annotator:span:layer:label:version，同一标注员对同一版本重复投票
// 只会得到既有标注，不会产生第二条记录。
package annotation

import (
	"fmt"
	"strings"

	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/store"
)

// Service 提供标注业务操作。
type Service struct {
	anns  *store.AnnotationStore
	spans *store.CorpusStore
}

// NewService 创建标注服务。
func NewService(a *store.AnnotationStore, s *store.CorpusStore) *Service {
	return &Service{anns: a, spans: s}
}

// Draft 创建草稿标注。重复投票（同 annotator+span+layer+label+version）幂等返回既有标注。
func (s *Service) Draft(spanID int64, annotator string, layer model.Layer, label string,
	start, end int, versionID int64) (*model.Annotation, error) {
	if err := s.validate(spanID, annotator, layer, label, start, end); err != nil {
		return nil, err
	}
	voteKey := VoteKey(annotator, spanID, layer, label, versionID)
	a := &model.Annotation{
		SpanID: spanID, Annotator: annotator, Layer: layer, Label: label,
		StartOffset: start, EndOffset: end, GuidelineVerID: versionID,
		Status: model.AnnDraft, VoteKey: voteKey,
	}
	id, err := s.anns.Create(a)
	if err == model.ErrDuplicate {
		// 幂等：返回既有同键标注。
		existing, gerr := s.anns.GetByVoteKey(voteKey)
		if gerr != nil {
			return nil, gerr
		}
		return existing, nil
	}
	if err != nil {
		return nil, err
	}
	a.ID = id
	return a, nil
}

// Submit 提交草稿标注（draft→submitted）。
func (s *Service) Submit(id int64) (*model.Annotation, error) {
	if err := s.anns.Submit(id, store.Now()); err != nil {
		return nil, err
	}
	return s.anns.Get(id)
}

// UpdateDraft 更新草稿标注的标签与边界。
func (s *Service) UpdateDraft(id int64, label string, start, end int) (*model.Annotation, error) {
	a, err := s.anns.Get(id)
	if err != nil {
		return nil, err
	}
	if err := s.validate(a.SpanID, a.Annotator, a.Layer, label, start, end); err != nil {
		return nil, err
	}
	if err := s.anns.UpdateDraft(id, label, start, end); err != nil {
		return nil, err
	}
	return s.anns.Get(id)
}

// ListBySpan 列出片段上的标注。
func (s *Service) ListBySpan(spanID int64, layer string) ([]model.Annotation, error) {
	return s.anns.ListBySpan(spanID, layer)
}

// MarkRereview 将已采纳标注标记为待重审（准则更新触发）。
func (s *Service) MarkRereview(id int64) error { return s.anns.MarkRereview(id, store.Now()) }

// VoteKey 构造幂等键。
func VoteKey(annotator string, spanID int64, layer model.Layer, label string, versionID int64) string {
	return fmt.Sprintf("%s:%d:%s:%s:%d", annotator, spanID, layer, label, versionID)
}

// validate 校验标注入参：标注员非空、层合法、标签非空、边界合法。
func (s *Service) validate(spanID int64, annotator string, layer model.Layer, label string, start, end int) error {
	annotator = strings.TrimSpace(annotator)
	if annotator == "" {
		return fmt.Errorf("%w: annotator required", model.ErrInvalidInput)
	}
	if !model.ValidLayer(layer) {
		return fmt.Errorf("%w: unsupported layer %q", model.ErrInvalidInput, layer)
	}
	if strings.TrimSpace(label) == "" {
		return fmt.Errorf("%w: label required", model.ErrInvalidInput)
	}
	sp, err := s.spans.GetSpan(spanID)
	if err != nil {
		return err
	}
	// 草稿更新沿用片段边界校验，冻结状态由上层工作流负责。
	// 越界边界拒绝：标签边界必须落在片段内部。
	if start < sp.StartOffset || end > sp.EndOffset || end <= start {
		return fmt.Errorf("%w: label offsets [%d,%d) outside span [%d,%d)",
			model.ErrInvalidInput, start, end, sp.StartOffset, sp.EndOffset)
	}
	return nil
}
