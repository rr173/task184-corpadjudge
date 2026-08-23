package baseline

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/store"
)

// fp 计算 sha256(doc_id:start:end:text) 指纹，与 corpus.Fingerprint 保持一致。
func fp(docID string, start, end int, text string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s:%d:%d:%s", docID, start, end, text)
	return hex.EncodeToString(h.Sum(nil))
}

// seedRereviewWithCase 构造最小数据：一个已裁决案例 + 一条待解决重审任务。
func seedRereviewWithCase(t *testing.T) (*Rereview, int64, int64) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	astore := store.NewAdjudicationStore(db)
	bstore := store.NewBaselineStore(db)
	cstore := store.NewCorpusStore(db)
	dstore := store.NewDisputeStore(db)

	sp, err := cstore.CreateSpan(&model.CorpusSpan{
		DocID: "doc1", StartOffset: 0, EndOffset: 4, Text: "测试",
		Fingerprint: fp("doc1", 0, 4, "测试"), Layer: model.LayerPOS,
		Status: model.SpanPending, CreatedAt: store.Now(), UpdatedAt: store.Now(),
	})
	if err != nil {
		t.Fatalf("create span: %v", err)
	}
	did, err := dstore.Create(&model.Dispute{
		SpanID: sp, Layer: model.LayerPOS, LabelCount: 2, CreatedAt: store.Now(),
	})
	if err != nil {
		t.Fatalf("create dispute: %v", err)
	}

	// 裁决案例并推进到 decided。
	cid, err := astore.Create(&model.AdjudicationCase{
		DisputeID: did, SpanID: sp, Status: model.CasePending,
		ClauseIDs: []int64{1}, GuidelineVerID: 1, CreatedAt: store.Now(),
	})
	if err != nil {
		t.Fatalf("create case: %v", err)
	}
	if err := astore.Decide(cid, "V", "rationale", []int64{1}, store.Now()); err != nil {
		t.Fatalf("decide case: %v", err)
	}

	rr := NewRereview(bstore, astore)
	task, err := rr.Create(cid, 1, 2, "guideline update")
	if err != nil {
		t.Fatalf("create rereview: %v", err)
	}
	return rr, task.ID, cid
}

// TestResolveRereviewWithoutReplacementCase 验证修复：当声称生成新决定但替换案例不存在时，
// 重审任务不得被关闭，旧案例也不得被标记为已替代。
func TestResolveRereviewWithoutReplacementCase(t *testing.T) {
	rr, taskID, oldCaseID := seedRereviewWithCase(t)

	// newLabel 非空且 newCaseID 指向一个不存在的案例。
	if _, err := rr.Resolve(taskID, "新决定", "V", 99999); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("resolve with missing replacement: err=%v, want ErrNotFound", err)
	}

	// 关键断言：重审任务仍为 open。
	open, err := rr.List("open", 100, 0)
	if err != nil {
		t.Fatalf("list open: %v", err)
	}
	if len(open) != 1 || open[0].ID != taskID {
		t.Fatalf("expected rereview %d still open, got %d tasks", taskID, len(open))
	}

	// 旧案例未被标记为已替代（决定未丢失）。
	c, err := rr.astore.Get(oldCaseID)
	if err != nil {
		t.Fatalf("get old case: %v", err)
	}
	if c.Status != model.CaseDecided {
		t.Fatalf("old case status = %q, want decided (not superseded)", c.Status)
	}
}
