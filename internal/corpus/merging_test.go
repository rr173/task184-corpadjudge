package corpus

import (
	"testing"

	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/store"
)

// newMergerTestDB 打开内存数据库并组装归并器所需三个存储。
func newMergerTestDB(t *testing.T) (*store.CorpusStore, *store.AnnotationStore, *store.DisputeStore) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return store.NewCorpusStore(db), store.NewAnnotationStore(db), store.NewDisputeStore(db)
}

// TestMergeIgnoresUnsubmittedDraft 验证归并不把未提交草稿计入片段状态：
// 存在草稿但尚未提交时，归并应视为无有效标注，片段维持 pending，不创建分歧。
func TestMergeIgnoresUnsubmittedDraft(t *testing.T) {
	cstore, astore, dstore := newMergerTestDB(t)
	csvc := NewService(cstore)
	m := NewMerger(cstore, astore, dstore)

	sp, err := csvc.Create("doc-draft", 0, 4, "测试文本", model.LayerPOS)
	if err != nil {
		t.Fatalf("create span: %v", err)
	}

	// 草稿标注：直接写库，不调用 Submit，模拟尚未提交的草稿。
	if _, err := astore.Create(&model.Annotation{
		SpanID: sp.ID, Annotator: "ann-a", Layer: model.LayerPOS, Label: "N",
		StartOffset: 0, EndOffset: 2, GuidelineVerID: 1,
		Status: model.AnnDraft, VoteKey: "ann-a:1:pos:N:1",
	}); err != nil {
		t.Fatalf("create draft annotation: %v", err)
	}

	res, err := m.MergeSpan(sp.ID, model.LayerPOS)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if res.Consistent {
		t.Fatalf("draft-only span should not be consistent: %+v", res)
	}
	if len(res.LabelGroups) != 0 {
		t.Fatalf("draft labels must not be counted, got label_groups=%v", res.LabelGroups)
	}
	if res.DisputeID != 0 {
		t.Fatalf("no dispute should be created for draft-only span, got dispute=%d", res.DisputeID)
	}
	got, _ := csvc.Get(sp.ID)
	if got.Status != model.SpanPending {
		t.Fatalf("span status=%q, want pending (draft must not flip span state)", got.Status)
	}
}

// TestMergeCountsSubmittedOnly 验证已提交标注参与归并：
// 单一已提交标签 → 一致；草稿即便同标签也不额外计入票数。
func TestMergeCountsSubmittedOnly(t *testing.T) {
	cstore, astore, dstore := newMergerTestDB(t)
	csvc := NewService(cstore)
	m := NewMerger(cstore, astore, dstore)

	sp, err := csvc.Create("doc-sub", 0, 4, "测试文本", model.LayerPOS)
	if err != nil {
		t.Fatalf("create span: %v", err)
	}
	// 一条已提交标注。
	if _, err := astore.Create(&model.Annotation{
		SpanID: sp.ID, Annotator: "ann-a", Layer: model.LayerPOS, Label: "N",
		StartOffset: 0, EndOffset: 2, GuidelineVerID: 1,
		Status: model.AnnSubmitted, VoteKey: "ann-a:2:pos:N:1", SubmittedAt: store.Now(),
	}); err != nil {
		t.Fatalf("create submitted annotation: %v", err)
	}
	// 同标签的草稿：不应额外加票，也不应改变一致判定。
	if _, err := astore.Create(&model.Annotation{
		SpanID: sp.ID, Annotator: "ann-b", Layer: model.LayerPOS, Label: "N",
		StartOffset: 0, EndOffset: 2, GuidelineVerID: 1,
		Status: model.AnnDraft, VoteKey: "ann-b:3:pos:N:1",
	}); err != nil {
		t.Fatalf("create draft annotation: %v", err)
	}

	res, err := m.MergeSpan(sp.ID, model.LayerPOS)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !res.Consistent {
		t.Fatalf("single submitted label should be consistent: %+v", res)
	}
	if got := res.LabelGroups["N"]; got != 1 {
		t.Fatalf("draft must not be counted: N votes=%d, want 1", got)
	}
	got, _ := csvc.Get(sp.ID)
	if got.Status != model.SpanConsistent {
		t.Fatalf("span status=%q, want consistent", got.Status)
	}
}
