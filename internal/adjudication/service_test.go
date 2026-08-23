package adjudication

import (
	"errors"
	"testing"

	"task184-corpadjudge/internal/model"
)

func TestCompileRuleAndSnapshotableGuards(t *testing.T) {
	c := &model.AdjudicationCase{ID: 9, SpanID: 4, DecidedLabel: "V", ClauseIDs: []int64{2, 5}, Status: model.CaseFormalized}
	rule := CompileRule(c)
	if rule == "" || rule != `IF span_id=4 AND layer conflict THEN label="V" (cited C2,C5)` {
		t.Fatalf("compiled rule=%q", rule)
	}
	detector := &ConflictDetector{}
	if err := detector.Snapshotable(c); err != nil {
		t.Fatalf("formalized case should be snapshotable: %v", err)
	}
	c.BaselineID = 11
	if err := detector.Snapshotable(c); !errors.Is(err, model.ErrDuplicate) {
		t.Fatalf("bound case error=%v, want ErrDuplicate", err)
	}
}

// TestCreateRejectsNoClauses 裁决案例在未引用任何准则条款时不应被创建。
// 条款校验先于任何 store 访问，故空条款直接返回 ErrInvalidInput。
func TestCreateRejectsNoClauses(t *testing.T) {
	svc := &Service{}
	for _, clauses := range [][]int64{nil, {}} {
		_, err := svc.Create(1, 1, clauses)
		if !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("create with clauses=%v: err=%v, want ErrInvalidInput", clauses, err)
		}
	}
}
