package guideline

import (
	"errors"
	"testing"

	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/store"
)

func TestGuidelineLifecycleGuardsClauseEdits(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := NewService(store.NewGuidelineStore(db))
	v, err := svc.CreateVersion("准则", "desc", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddClause(v.ID, "C1", model.LayerPOS, "词性", "按语境", "中文"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Publish(v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddClause(v.ID, "C2", model.LayerPOS, "禁止编辑", "x", ""); !errors.Is(err, model.ErrInvalidState) {
		t.Fatalf("editing published guideline error=%v, want ErrInvalidState", err)
	}
	if _, err := svc.MarkAmbiguous(v.ID, "需要澄清"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Publish(v.ID); err != nil {
		t.Fatal(err)
	}
	final, err := svc.Revoke(v.ID, "废止旧版本")
	if err != nil || final.Status != model.GuidelineRevoked {
		t.Fatalf("revoke status=%q err=%v", final.Status, err)
	}
}
