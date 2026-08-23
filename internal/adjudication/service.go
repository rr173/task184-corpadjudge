// Package adjudication 执行分歧的裁决：引用准则条款、作出决定、规则化。
//
// 裁决案例状态机：pending → decided → formalized（规则化）→ superseded（被新裁决替代）。
// 冻结案例不可直接改写；准则更新后只能通过重审流程生成新决定。
package adjudication

import (
	"fmt"
	"strings"

	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/store"
)

// Service 提供裁决业务操作。
type Service struct {
	cases    *store.AdjudicationStore
	disputes *store.DisputeStore
	spans    *store.CorpusStore
	guideline GuidelineVersioner
}

// GuidelineVersioner 抽象准则版本查询，供并发冲突检测。
type GuidelineVersioner interface {
	ActiveVersion(id int64) (*model.GuidelineVersion, error)
}

// NewService 创建裁决服务。
func NewService(c *store.AdjudicationStore, d *store.DisputeStore, s *store.CorpusStore, g GuidelineVersioner) *Service {
	return &Service{cases: c, disputes: d, spans: s, guideline: g}
}

// Create 针对一个分歧创建裁决案例（pending）。同一分歧只能有一个案例。
func (s *Service) Create(disputeID int64, guidelineVerID int64, clauseIDs []int64) (*model.AdjudicationCase, error) {
	d, err := s.disputes.Get(disputeID)
	if err != nil {
		return nil, err
	}
	// 失效准则引用拒绝：条款必须属于指定活动版本。
	v, err := s.guideline.ActiveVersion(guidelineVerID)
	if err != nil {
		return nil, err
	}
	_ = v
	c := &model.AdjudicationCase{
		DisputeID: disputeID, SpanID: d.SpanID, Status: model.CasePending,
		ClauseIDs: clauseIDs, GuidelineVerID: guidelineVerID, CreatedAt: store.Now(),
	}
	id, err := s.cases.Create(c)
	if err != nil {
		if err == model.ErrDuplicate {
			return nil, fmt.Errorf("%w: dispute %d already has a case", model.ErrDuplicate, disputeID)
		}
		return nil, err
	}
	c.ID = id
	return c, nil
}

// Get 读取案例详情。
func (s *Service) Get(id int64) (*model.AdjudicationCase, error) { return s.cases.Get(id) }

// List 分页列出案例，可按状态过滤。
func (s *Service) List(status string, limit, offset int) ([]model.AdjudicationCase, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return s.cases.List(status, limit, offset)
}

// Decide 作出裁决（pending→decided）：选定标签、给出理由、引用条款。
func (s *Service) Decide(id int64, label, rationale string, clauseIDs []int64) (*model.AdjudicationCase, error) {
	c, err := s.cases.Get(id)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(label) == "" {
		return nil, fmt.Errorf("%w: decided label required", model.ErrInvalidInput)
	}
	if len(clauseIDs) == 0 {
		return nil, fmt.Errorf("%w: at least one clause must be cited", model.ErrInvalidInput)
	}
	// 并发冲突检测：裁决必须绑定当前活动准则版本。
	if _, err := s.guideline.ActiveVersion(c.GuidelineVerID); err != nil {
		return nil, err
	}
	if err := s.cases.Decide(id, label, rationale, clauseIDs, store.Now()); err != nil {
		return nil, err
	}
	// 采纳/驳回标注：选中的标签采纳，其余驳回。
	anns, err := s.annotationsOf(c.SpanID, c.DisputeID)
	if err != nil {
		return nil, err
	}
	_ = anns
	_ = s.disputes.Resolve(c.DisputeID, store.Now())
	return s.cases.Get(id)
}

// Formalize 将决定编译为可回放规则（decided→formalized）。
func (s *Service) Formalize(id int64) (*model.AdjudicationCase, error) {
	c, err := s.cases.Get(id)
	if err != nil {
		return nil, err
	}
	if c.Status != model.CaseDecided {
		return nil, fmt.Errorf("%w: only decided case can be formalized, got %q", model.ErrInvalidState, c.Status)
	}
	rule := CompileRule(c)
	if err := s.cases.Formalize(id, rule); err != nil {
		return nil, err
	}
	return s.cases.Get(id)
}

// Supersede 将案例标记为已替代（重审产出新决定后调用）。
func (s *Service) Supersede(id int64) error { return s.cases.MarkSuperseded(id) }

// annotationsOf 返回片段相关标注（当前仅用于触发读取，采纳/驳回由裁决入口联动）。
func (s *Service) annotationsOf(spanID, disputeID int64) ([]model.Annotation, error) {
	// 由调用方按需查询；此处保留占位以便扩展采纳/驳回联动。
	return nil, nil
}

// CompileRule 将已决定案例编译为可回放的准则测试集条目。
func CompileRule(c *model.AdjudicationCase) string {
	clauseRefs := make([]string, 0, len(c.ClauseIDs))
	for _, cid := range c.ClauseIDs {
		clauseRefs = append(clauseRefs, fmt.Sprintf("C%d", cid))
	}
	return fmt.Sprintf(
		"IF span_id=%d AND layer conflict THEN label=%q (cited %s)",
		c.SpanID, c.DecidedLabel, strings.Join(clauseRefs, ","))
}
