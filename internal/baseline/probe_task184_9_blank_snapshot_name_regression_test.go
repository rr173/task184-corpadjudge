package baseline

import (
	"errors"
	"testing"

	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/store"
)

func TestBlankSnapshotNameIsRejected(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := NewService(store.NewBaselineStore(db), store.NewAdjudicationStore(db), store.NewCorpusStore(db))
	if _, err := svc.CreateSnapshot("   ", 1); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("blank snapshot name error=%v, want ErrInvalidInput", err)
	}
}
