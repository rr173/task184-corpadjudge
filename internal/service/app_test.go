package service

import (
	"path/filepath"
	"testing"

	"task184-corpadjudge/internal/dispute"
	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/store"
)

// newTestApp 创建内存数据库的服务实例。
func newTestApp(t *testing.T) *App {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	app, err := New(db)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	return app
}

// TestWorkflowDisputeAndAdjudicate 端到端：准则→片段→三人标注→分歧→裁决→冻结→影响分析。
func TestWorkflowDisputeAndAdjudicate(t *testing.T) {
	app := newTestApp(t)

	v, err := app.Guideline.CreateVersion("准则A", "desc", "v1")
	if err != nil {
		t.Fatalf("create version: %v", err)
	}
	c1, err := app.Guideline.AddClause(v.ID, "C1", model.LayerCoref, "指代边界", "边界落在名词短语边界", "核心指代")
	if err != nil {
		t.Fatalf("add clause: %v", err)
	}
	if _, err := app.Guideline.Publish(v.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}

	sp, err := app.Corpus.Create("doc1", 0, 6, "小明去公司", model.LayerCoref)
	if err != nil {
		t.Fatalf("create span: %v", err)
	}

	// 三人两种标签 → 分歧。
	for _, vote := range []struct {
		who, label string
	}{{"a", "B-PER"}, {"b", "B-LOC"}, {"c", "B-PER"}} {
		if _, _, err := app.Workflow(sp.ID, vote.who, model.LayerCoref, vote.label, 0, 2, v.ID); err != nil {
			t.Fatalf("workflow %s: %v", vote.who, err)
		}
	}

	disputes, err := app.Dispute.List(sp.ID, 10, 0)
	if err != nil || len(disputes) != 1 {
		t.Fatalf("expected 1 dispute, got %d (err=%v)", len(disputes), err)
	}
	matrix, err := app.Dispute.Matrix(disputes[0].ID)
	if err != nil || len(matrix) != 2 {
		t.Fatalf("expected 2 matrix labels, got %d (err=%v)", len(matrix), err)
	}

	_, snap, err := app.AdjudicateAndFreeze(disputes[0].ID, v.ID, "B-PER", "名词短语", []int64{c1.ID}, "基准-1")
	if err != nil {
		t.Fatalf("adjudicate+freeze: %v", err)
	}
	if !snap.Frozen {
		t.Fatal("snapshot not frozen")
	}
	gotSpan, _ := app.Corpus.Get(sp.ID)
	if gotSpan.Status != model.SpanFrozen {
		t.Fatalf("span status = %q, want frozen", gotSpan.Status)
	}
}

// TestIdempotentVoteAndFingerprint 幂等：重复投票与重复片段均不产生重复数据。
func TestIdempotentVoteAndFingerprint(t *testing.T) {
	app := newTestApp(t)

	v, _ := app.Guideline.CreateVersion("准则A", "", "")
	if _, err := app.Guideline.Publish(v.ID); err != nil {
		t.Fatal(err)
	}
	sp, err := app.Corpus.Create("doc1", 0, 4, "测试文本", model.LayerPOS)
	if err != nil {
		t.Fatal(err)
	}
	// 指纹幂等。
	sp2, err := app.Corpus.Create("doc1", 0, 4, "测试文本", model.LayerPOS)
	if err != nil || sp2.ID != sp.ID {
		t.Fatalf("fingerprint dedup failed: err=%v id1=%d id2=%d", err, sp.ID, sp2.ID)
	}
	// 投票幂等。
	if _, _, err := app.Workflow(sp.ID, "ann", model.LayerPOS, "N", 0, 2, v.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := app.Workflow(sp.ID, "ann", model.LayerPOS, "N", 0, 2, v.ID); err != nil {
		t.Fatalf("resubmit same vote: %v", err)
	}
	anns, _ := app.Annotation.ListBySpan(sp.ID, string(model.LayerPOS))
	n := 0
	for _, a := range anns {
		if a.Annotator == "ann" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("idempotent vote broken: ann has %d records", n)
	}
}

// TestCrossLayerGuard 层级混淆防护：互斥层被拒绝。
func TestCrossLayerGuard(t *testing.T) {
	app := newTestApp(t)

	v, _ := app.Guideline.CreateVersion("准则A", "", "")
	if _, err := app.Guideline.Publish(v.ID); err != nil {
		t.Fatal(err)
	}
	sp, _ := app.Corpus.Create("doc1", 0, 4, "测试文本", model.LayerNamedEnt)
	if _, _, err := app.Workflow(sp.ID, "ann", model.LayerNamedEnt, "B-PER", 0, 2, v.ID); err != nil {
		t.Fatal(err)
	}
	// 同一标注员再标指代层 → 应被互斥规则拒绝。
	if _, _, err := app.Workflow(sp.ID, "ann", model.LayerCoref, "X", 0, 2, v.ID); err == nil {
		t.Fatal("expected cross-layer guard to reject")
	}
}

// TestMatrixDedupsAnnotatorAcrossVersions 同一标注员在两个准则版本下提交相同
// 标签（vote_key 不同 → 两条 submitted 标注行）时，分歧矩阵与汇总不得把该标注员
// 重复计入票数与人数。
func TestMatrixDedupsAnnotatorAcrossVersions(t *testing.T) {
	app := newTestApp(t)

	v1, _ := app.Guideline.CreateVersion("准则A", "", "")
	if _, err := app.Guideline.Publish(v1.ID); err != nil {
		t.Fatal(err)
	}
	sp, err := app.Corpus.Create("doc1", 0, 4, "测试文本", model.LayerPOS)
	if err != nil {
		t.Fatal(err)
	}
	// 两位标注员两种标签 → 分歧。
	if _, _, err := app.Workflow(sp.ID, "ann-a", model.LayerPOS, "V", 0, 2, v1.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := app.Workflow(sp.ID, "ann-b", model.LayerPOS, "N", 0, 2, v1.ID); err != nil {
		t.Fatal(err)
	}
	// ann-a 在新版本下再次提交同标签：vote_key 不同 → 产生第二条 submitted 行。
	v2, _ := app.Guideline.CreateVersion("准则A更新", "", "")
	if _, err := app.Guideline.Publish(v2.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := app.Workflow(sp.ID, "ann-a", model.LayerPOS, "V", 0, 2, v2.ID); err != nil {
		t.Fatal(err)
	}

	disputes, err := app.Dispute.List(sp.ID, 10, 0)
	if err != nil || len(disputes) != 1 {
		t.Fatalf("expected 1 dispute, got %d (err=%v)", len(disputes), err)
	}
	matrix, err := app.Dispute.Matrix(disputes[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	// 每个标签的 VoteCount 应等于去重后的标注员数，而非标注行数。
	for _, e := range matrix {
		if e.Label == "V" && e.VoteCount != 1 {
			t.Fatalf("V VoteCount=%d want 1 (ann-a deduped)", e.VoteCount)
		}
	}
	sum := dispute.Summarize(matrix)
	if sum.DistinctAnnotators != 2 {
		t.Fatalf("DistinctAnnotators=%d want 2", sum.DistinctAnnotators)
	}
	if sum.TotalVotes != 2 {
		t.Fatalf("TotalVotes=%d want 2", sum.TotalVotes)
	}
}

// TestFrozenCaseConflict 冻结案例不可直接改写，冲突守卫应拦截。
func TestFrozenCaseConflict(t *testing.T) {
	app := newTestApp(t)

	v, _ := app.Guideline.CreateVersion("准则A", "", "")
	c1, _ := app.Guideline.AddClause(v.ID, "C1", model.LayerSemRole, "论元", "角色判定", "语义角色")
	if _, err := app.Guideline.Publish(v.ID); err != nil {
		t.Fatal(err)
	}
	sp, _ := app.Corpus.Create("doc1", 0, 4, "测试文本", model.LayerSemRole)
	for _, vote := range []struct{ who, label string }{{"a", "AGENT"}, {"b", "THEME"}} {
		if _, _, err := app.Workflow(sp.ID, vote.who, model.LayerSemRole, vote.label, 0, 2, v.ID); err != nil {
			t.Fatal(err)
		}
	}
	ds, _ := app.Dispute.List(sp.ID, 5, 0)
	decided, _, err := app.AdjudicateAndFreeze(ds[0].ID, v.ID, "AGENT", "role", []int64{c1.ID}, "基准-1")
	if err != nil {
		t.Fatalf("adjudicate: %v", err)
	}
	if err := app.Conflict.Check(decided.ID); err == nil {
		t.Fatal("expected conflict for frozen case, got nil")
	}
}

// TestPersistenceRestore 持久化恢复：关闭数据库重开后数据仍在。
func TestPersistenceRestore(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")

	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := app.Guideline.CreateVersion("准则A", "", "")
	if _, err := app.Guideline.Publish(v.ID); err != nil {
		t.Fatal(err)
	}
	sp, _ := app.Corpus.Create("doc1", 0, 4, "测试文本", model.LayerPOS)
	if _, _, err := app.Workflow(sp.ID, "ann", model.LayerPOS, "N", 0, 2, v.ID); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db2, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	app2, err := New(db2)
	if err != nil {
		t.Fatal(err)
	}
	gotV, err := app2.Guideline.Get(v.ID)
	if err != nil || gotV.Status != model.GuidelinePublished {
		t.Fatalf("version not restored: err=%v", err)
	}
	gotSpan, err := app2.Corpus.Get(sp.ID)
	if err != nil || gotSpan.Status != model.SpanConsistent {
		t.Fatalf("span not restored: err=%v status=%q", err, gotSpan.Status)
	}
}
