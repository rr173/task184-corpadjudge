package adjudication_test

import (
	"errors"
	"testing"

	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/service"
	"task184-corpadjudge/internal/store"
)

func TestAdjudicationCreateRequiresClauseReferences(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app, err := service.New(db)
	if err != nil {
		t.Fatal(err)
	}
	v, err := app.Guideline.CreateVersion("case-clause", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Guideline.Publish(v.ID); err != nil {
		t.Fatal(err)
	}
	sp, err := app.Corpus.Create("doc-case", 0, 4, "测试文本", model.LayerPOS)
	if err != nil {
		t.Fatal(err)
	}
	ds := store.NewDisputeStore(db)
	d := &model.Dispute{SpanID: sp.ID, Layer: model.LayerPOS, Status: "open", LabelCount: 2, CreatedAt: store.Now()}
	d.ID, err = ds.Create(d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Adjudication.Create(d.ID, v.ID, nil); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("create without clauses error=%v, want ErrInvalidInput", err)
	}
}
