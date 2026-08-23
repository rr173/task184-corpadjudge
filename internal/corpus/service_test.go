package corpus

import (
	"errors"
	"testing"

	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/store"
)

func TestFingerprintCreateIsIdempotentAndValidatesBounds(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := NewService(store.NewCorpusStore(db))
	first, err := svc.Create("doc-1", 0, 4, "测试文本", model.LayerPOS)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Create("doc-1", 0, 4, "测试文本", model.LayerPOS)
	if err != nil || second.ID != first.ID || second.Fingerprint != Fingerprint("doc-1", 0, 4, "测试文本") {
		t.Fatalf("duplicate create first=%+v second=%+v err=%v", first, second, err)
	}
	if _, err := svc.Create("doc-1", -1, 2, "测试", model.LayerPOS); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("invalid offsets error=%v, want ErrInvalidInput", err)
	}
	if _, err := svc.Freeze(first.ID); !errors.Is(err, model.ErrInvalidState) {
		t.Fatalf("freezing pending span error=%v, want ErrInvalidState", err)
	}
}
