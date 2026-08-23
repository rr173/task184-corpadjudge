// Package corpus 管理语料片段：创建、指纹幂等、状态推进。
//
// 片段状态机：pending（待标注）→ consistent（一致）/ disputed（存在分歧）
// → adjudicated（已裁决）→ frozen（已冻结，绑定原准则版本）。
package corpus

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/store"
)

// Service 提供语料片段业务操作。
type Service struct {
	store *store.CorpusStore
}

// NewService 创建片段服务。
func NewService(s *store.CorpusStore) *Service { return &Service{store: s} }

// Fingerprint 计算片段指纹：sha256(doc_id:start:end:text)。
// 相同指纹视为同一片段，重复创建时幂等返回既有片段。
func Fingerprint(docID string, start, end int, text string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s:%d:%d:%s", docID, start, end, text)
	return hex.EncodeToString(h.Sum(nil))
}

// Create 创建片段。校验边界合法性；指纹重复时幂等返回既有片段。
func (s *Service) Create(docID string, start, end int, text string, layer model.Layer) (*model.CorpusSpan, error) {
	docID = strings.TrimSpace(docID)
	if docID == "" || text == "" {
		return nil, fmt.Errorf("%w: doc_id and text required", model.ErrInvalidInput)
	}
	if !model.ValidLayer(layer) {
		return nil, fmt.Errorf("%w: unsupported layer %q", model.ErrInvalidInput, layer)
	}
	if start < 0 || end <= start || end > start+len(text) {
		return nil, fmt.Errorf("%w: invalid offsets [%d,%d) for text length %d", model.ErrInvalidInput, start, end, len(text))
	}
	fp := Fingerprint(docID, start, end, text)
	if existing, err := s.store.GetSpanByFingerprint(fp); err == nil {
		if existing.Status == model.SpanFrozen {
			return nil, model.ErrDuplicate
		}
		return existing, nil // 幂等：重复提交返回既有片段
	}
	now := store.Now()
	sp := &model.CorpusSpan{
		DocID: docID, StartOffset: start, EndOffset: end, Text: text,
		Fingerprint: fp, Layer: layer, Status: model.SpanPending,
		CreatedAt: now, UpdatedAt: now,
	}
	id, err := s.store.CreateSpan(sp)
	if err != nil {
		if err == model.ErrDuplicate {
			if existing, gerr := s.store.GetSpanByFingerprint(fp); gerr == nil {
				return existing, nil
			}
		}
		return nil, err
	}
	sp.ID = id
	return sp, nil
}

// Get 按 ID 读取片段。
func (s *Service) Get(id int64) (*model.CorpusSpan, error) { return s.store.GetSpan(id) }

// List 分页列出片段，可按状态过滤。
func (s *Service) List(status string, limit, offset int) ([]model.CorpusSpan, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return s.store.ListSpans(status, limit, offset)
}

// MarkStatus 推进片段状态（pending→consistent/disputed/adjudicated）。
func (s *Service) MarkStatus(id int64, status string) error {
	switch status {
	case model.SpanConsistent, model.SpanDisputed, model.SpanAdjudicated:
	default:
		return fmt.Errorf("%w: unsupported span status %q", model.ErrInvalidInput, status)
	}
	return s.store.UpdateSpanStatus(id, status, store.Now())
}

// Freeze 冻结片段（仅允许从 adjudicated/consistent 冻结）。
func (s *Service) Freeze(id int64) (*model.CorpusSpan, error) {
	sp, err := s.store.GetSpan(id)
	if err != nil {
		return nil, err
	}
	switch sp.Status {
	case model.SpanAdjudicated, model.SpanConsistent:
	default:
		return nil, fmt.Errorf("%w: cannot freeze span in %q", model.ErrInvalidState, sp.Status)
	}
	if err := s.store.FreezeSpan(id, store.Now()); err != nil {
		return nil, err
	}
	return s.store.GetSpan(id)
}
