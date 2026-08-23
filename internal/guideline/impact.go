package guideline

import (
	"fmt"

	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/store"
)

// ImpactReport 是准则影响分析的结果：新准则版本发布后，
// 旧基准集中哪些案例需要重审，哪些条款被改写或废止。
type ImpactReport struct {
	FromVersionID    int64             `json:"from_version_id"`
	ToVersionID      int64             `json:"to_version_id"`
	ChangedClauses   []ChangedClause   `json:"changed_clauses"`
	AffectedCases    []AffectedCase    `json:"affected_cases"`
	RereviewRequired int               `json:"rereview_required"`
	BaselinePreserved int              `json:"baseline_preserved"`
}

// ChangedClause 描述一次条款变更。
type ChangedClause struct {
	ClauseNo   string    `json:"clause_no"`
	Layer      model.Layer `json:"layer"`
	Title      string    `json:"title"`
	ChangeType string    `json:"change_type"` // rewritten / revoked / added
}

// AffectedCase 描述受影响的裁决案例。
type AffectedCase struct {
	CaseID         int64  `json:"case_id"`
	SpanID         int64  `json:"span_id"`
	DecidedLabel   string `json:"decided_label"`
	BaselineID     int64  `json:"baseline_id"`
	Reason         string `json:"reason"`
}

// ImpactAnalyzer 分析准则版本更新对既有基准的影响。
type ImpactAnalyzer struct {
	gstore *store.GuidelineStore
	astore *store.AdjudicationStore
	bstore *store.BaselineStore
}

// NewImpactAnalyzer 创建影响分析器。
func NewImpactAnalyzer(g *store.GuidelineStore, a *store.AdjudicationStore, b *store.BaselineStore) *ImpactAnalyzer {
	return &ImpactAnalyzer{gstore: g, astore: a, bstore: b}
}

// Analyze 对比 fromVersion 与 toVersion 的条款差异，
// 并扫描引用 fromVersion 的已决定案例，产出重审清单。
func (ia *ImpactAnalyzer) Analyze(fromID, toID int64) (*ImpactReport, error) {
	fromV, err := ia.gstore.GetVersion(fromID)
	if err != nil {
		return nil, err
	}
	if _, err := ia.gstore.GetVersion(toID); err != nil {
		return nil, err
	}
	report := &ImpactReport{
		FromVersionID: fromID,
		ToVersionID:   toID,
	}

	fromClauses, err := ia.gstore.ListClauses(fromID)
	if err != nil {
		return nil, err
	}
	toClauses, err := ia.gstore.ListClauses(toID)
	if err != nil {
		return nil, err
	}
	toByNo := map[string]model.GuidelineClause{}
	for _, c := range toClauses {
		toByNo[c.ClauseNo] = c
	}
	fromByNo := map[string]model.GuidelineClause{}
	for _, c := range fromClauses {
		fromByNo[c.ClauseNo] = c
	}

	// 条款差异：新增 / 改写 / 废止。
	for _, c := range toClauses {
		old, ok := fromByNo[c.ClauseNo]
		if !ok {
			report.ChangedClauses = append(report.ChangedClauses, ChangedClause{c.ClauseNo, c.Layer, c.Title, "added"})
			continue
		}
		if old.Body != c.Body || old.Scope != c.Scope {
			report.ChangedClauses = append(report.ChangedClauses, ChangedClause{c.ClauseNo, c.Layer, c.Title, "rewritten"})
		}
	}
	for _, c := range fromClauses {
		if _, ok := toByNo[c.ClauseNo]; !ok {
			report.ChangedClauses = append(report.ChangedClauses, ChangedClause{c.ClauseNo, c.Layer, c.Title, "revoked"})
		}
	}

	// 引用 fromVersion 的已决定案例。
	cases, err := ia.astore.ListByVersion(fromID)
	if err != nil {
		return nil, err
	}
	for _, c := range cases {
		reason := ia.caseReason(c, toByNo)
		if reason == "" {
			continue // 未命中变更条款，无需重审
		}
		report.AffectedCases = append(report.AffectedCases, AffectedCase{
			CaseID: c.ID, SpanID: c.SpanID, DecidedLabel: c.DecidedLabel,
			BaselineID: c.BaselineID, Reason: reason,
		})
		report.RereviewRequired++
	}

	report.BaselinePreserved = ia.preservedBaselineCount(fromV)
	return report, nil
}

// caseReason 判断案例引用的条款是否落入变更集，返回重审原因（空串表示不受影响）。
func (ia *ImpactAnalyzer) caseReason(c model.AdjudicationCase, toByNo map[string]model.GuidelineClause) string {
	clauses, err := ia.gstore.ListClauses(c.GuidelineVerID)
	if err != nil {
		return "guideline clause lookup failed"
	}
	byID := map[int64]model.GuidelineClause{}
	for _, cl := range clauses {
		byID[cl.ID] = cl
	}
	for _, cid := range c.ClauseIDs {
		cl, ok := byID[cid]
		if !ok {
			continue
		}
		if _, still := toByNo[cl.ClauseNo]; !still {
			return fmt.Sprintf("引用的条款 %s 在新准则中被移除", cl.ClauseNo)
		}
		newCl := toByNo[cl.ClauseNo]
		if newCl.Body != cl.Body {
			return fmt.Sprintf("引用的条款 %s 正文已改写", cl.ClauseNo)
		}
		if newCl.Scope != cl.Scope {
			return fmt.Sprintf("引用的条款 %s 作用域已变更", cl.ClauseNo)
		}
	}
	return ""
}

// preservedBaselineCount 统计仍被旧版本基准保留的冻结案例数。
func (ia *ImpactAnalyzer) preservedBaselineCount(v *model.GuidelineVersion) int {
	snaps, err := ia.bstore.ListSnapshots()
	if err != nil {
		return 0
	}
	total := 0
	for _, s := range snaps {
		if s.GuidelineVerID == v.ID && s.Frozen {
			total += s.CaseCount
		}
	}
	return total
}
