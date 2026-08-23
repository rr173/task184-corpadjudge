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
