package annotation_test

import (
	"errors"
	"testing"

	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/service"
	"task184-corpadjudge/internal/store"
)

func TestFrozenSpanRejectsDraftAnnotationUpdate(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app, err := service.New(db)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := app.Guideline.CreateVersion("freeze-ann", "", "")
	if _, err := app.Guideline.Publish(v.ID); err != nil {
		t.Fatal(err)
	}
	sp, err := app.Corpus.Create("doc-ann", 0, 4, "测试文本", model.LayerPOS)
	if err != nil {
		t.Fatal(err)
	}
	ann, err := app.Annotation.Draft(sp.ID, "ann", model.LayerPOS, "N", 0, 2, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Corpus.MarkStatus(sp.ID, model.SpanConsistent); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Corpus.Freeze(sp.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Annotation.UpdateDraft(ann.ID, "V", 0, 2); !errors.Is(err, model.ErrFrozen) {
		t.Fatalf("frozen annotation update error=%v, want ErrFrozen", err)
	}
}
