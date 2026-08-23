package guideline_test

import (
	"testing"

	"task184-corpadjudge/internal/guideline"
	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/store"
)

func TestScopeOnlyClauseChangeAppearsInImpactReport(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	gs := store.NewGuidelineStore(db)
	as := store.NewAdjudicationStore(db)
	bs := store.NewBaselineStore(db)
	svc := guideline.NewService(gs)
	v1, err := svc.CreateVersion("impact", "", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddClause(v1.ID, "C1", model.LayerPOS, "词性", "body", "旧作用域"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Publish(v1.ID); err != nil {
		t.Fatal(err)
	}
	v2, err := svc.CreateVersion("impact", "", "v2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddClause(v2.ID, "C1", model.LayerPOS, "词性", "body", "新作用域"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Publish(v2.ID); err != nil {
		t.Fatal(err)
	}
	report, err := guideline.NewImpactAnalyzer(gs, as, bs).Analyze(v1.ID, v2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.ChangedClauses) != 1 || report.ChangedClauses[0].ChangeType != "rewritten" {
		t.Fatalf("changed clauses=%+v, want one rewritten clause", report.ChangedClauses)
	}
}
