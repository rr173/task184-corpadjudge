package corpus_test

import (
	"testing"

	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/service"
	"task184-corpadjudge/internal/store"
)

func TestDraftAnnotationDoesNotChangeSpanMergeState(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app, err := service.New(db)
	if err != nil {
		t.Fatal(err)
	}
	v, err := app.Guideline.CreateVersion("draft-merge", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Guideline.Publish(v.ID); err != nil {
		t.Fatal(err)
	}
	sp, err := app.Corpus.Create("doc-draft", 0, 4, "测试文本", model.LayerPOS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Annotation.Draft(sp.ID, "ann", model.LayerPOS, "N", 0, 2, v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Merger.MergeSpan(sp.ID, model.LayerPOS); err != nil {
		t.Fatal(err)
	}
	got, err := app.Corpus.Get(sp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.SpanPending {
		t.Fatalf("span status=%q after draft merge, want pending", got.Status)
	}
}
