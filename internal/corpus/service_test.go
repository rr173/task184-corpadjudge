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

// 冻结后再次提交相同指纹应幂等返回原片段，而非报冲突。
func TestCreateIsIdempotentAfterFreeze(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := NewService(store.NewCorpusStore(db))

	orig, err := svc.Create("doc-2", 0, 4, "测试文本", model.LayerPOS)
	if err != nil {
		t.Fatal(err)
	}
	// consistent 状态方可冻结。
	if err := svc.MarkStatus(orig.ID, model.SpanConsistent); err != nil {
		t.Fatal(err)
	}
	frozen, err := svc.Freeze(orig.ID)
	if err != nil {
		t.Fatalf("freeze: %v", err)
	}
	if frozen.Status != model.SpanFrozen {
		t.Fatalf("status=%q want frozen", frozen.Status)
	}

	// 再次提交相同指纹：应返回原片段，且不报 ErrDuplicate/冲突。
	again, err := svc.Create("doc-2", 0, 4, "测试文本", model.LayerPOS)
	if err != nil {
		t.Fatalf("resubmit after freeze error=%v, want nil", err)
	}
	if again.ID != orig.ID || again.Fingerprint != orig.Fingerprint || again.Status != model.SpanFrozen {
		t.Fatalf("resubmit not idempotent: orig=%+v again=%+v", orig, again)
	}
}
