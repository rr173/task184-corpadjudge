package baseline_test

import (
	"errors"
	"testing"

	"task184-corpadjudge/internal/baseline"
	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/store"
)

func TestRereviewResolveRejectsUnknownReplacementCase(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	bs := store.NewBaselineStore(db)
	as := store.NewAdjudicationStore(db)
	task := &model.RereviewTask{CaseID: 42, FromVersionID: 1, ToVersionID: 2, Reason: "changed", Status: "open", CreatedAt: store.Now()}
	id, err := bs.CreateRereview(task)
	if err != nil {
		t.Fatal(err)
	}
	r := baseline.NewRereview(bs, as)
	if _, err := r.Resolve(id, "replacement", "NEW", 999); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("resolve unknown replacement error=%v, want ErrNotFound", err)
	}
	open, err := bs.ListRereviews("open", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 {
		t.Fatalf("open rereviews=%d, want task left open", len(open))
	}
}
