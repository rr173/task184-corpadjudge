package guideline

import (
	"testing"

	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/store"
)

// newImpactAnalyzerFixture 在内存数据库上组装影响分析器及其依赖的存储。
func newImpactAnalyzerFixture(t *testing.T) (*ImpactAnalyzer, *store.DB) {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	gstore := store.NewGuidelineStore(db)
	astore := store.NewAdjudicationStore(db)
	bstore := store.NewBaselineStore(db)
	return NewImpactAnalyzer(gstore, astore, bstore), db
}

// newPublishedVersionWithClause 建一个已发布准则版本，附一条条款。
func newPublishedVersionWithClause(t *testing.T, svc *Service, name, clauseNo, body, scope string) (*model.GuidelineVersion, *model.GuidelineClause) {
	t.Helper()
	v, err := svc.CreateVersion(name, "desc", "note")
	if err != nil {
		t.Fatalf("create version: %v", err)
	}
	c, err := svc.AddClause(v.ID, clauseNo, model.LayerPOS, "条款", body, scope)
	if err != nil {
		t.Fatalf("add clause: %v", err)
	}
	v, err = svc.Publish(v.ID)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	return v, c
}

// createDecidedCaseWithClause 构造一条引用 clauseIDs 的已决定案例，供影响分析扫描。
func createDecidedCaseWithClause(t *testing.T, astore *store.AdjudicationStore, versionID, spanID, disputeID int64, clauseIDs []int64) *model.AdjudicationCase {
	t.Helper()
	c := &model.AdjudicationCase{
		DisputeID: disputeID, SpanID: spanID, Status: model.CasePending,
		ClauseIDs: clauseIDs, GuidelineVerID: versionID, CreatedAt: store.Now(),
	}
	id, err := astore.Create(c)
	if err != nil {
		t.Fatalf("create case: %v", err)
	}
	if err := astore.Decide(id, "V", "rationale", clauseIDs, store.Now()); err != nil {
		t.Fatalf("decide case: %v", err)
	}
	got, err := astore.Get(id)
	if err != nil {
		t.Fatalf("get case: %v", err)
	}
	return got
}

// TestScopeOnlyClauseChangeAppearsInImpactReport 验证当条款正文不变、仅作用域
// 发生变化时，影响分析仍须把该条款列入 ChangedClauses，并把引用它的案例标为
// 需重审（RereviewRequired）。修复前 ChangedClauses 仅比较 Body，会漏报这类条款。
func TestScopeOnlyClauseChangeAppearsInImpactReport(t *testing.T) {
	ia, _ := newImpactAnalyzerFixture(t)
	svc := NewService(ia.gstore)
	astore := ia.astore

	// v1：条款 C1 正文相同、作用域 A。
	v1, c1 := newPublishedVersionWithClause(t, svc, "准则", "C1", "兼类词按语境标注", "作用域-A")

	// 一条引用 C1 的已决定案例。
	c := createDecidedCaseWithClause(t, astore, v1.ID, 100, 200, []int64{c1.ID})

	// v2：同一条款号 C1，正文不变、仅作用域改为 B（仅作用域变化）。
	v2, _ := newPublishedVersionWithClause(t, svc, "准则", "C1", "兼类词按语境标注", "作用域-B")

	report, err := ia.Analyze(v1.ID, v2.ID)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}

	// 该条款应出现在变更条款清单中（修复前漏报）。
	var found bool
	for _, ch := range report.ChangedClauses {
		if ch.ClauseNo == "C1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("仅作用域变化的条款 C1 未出现在 ChangedClauses： %+v", report.ChangedClauses)
	}

	// 引用该条款的案例应被标为需重审。
	if report.RereviewRequired == 0 {
		t.Fatalf("引用仅作用域变化条款的案例 %d 未被标为需重审", c.ID)
	}
	if len(report.AffectedCases) == 0 {
		t.Fatalf("期望 AffectedCases 非空，得到 %+v", report.AffectedCases)
	}
}

// TestBodyOnlyClauseChangeInImpactReport 对照：正文变化但作用域不变时，
// 条款既出现在 ChangedClauses 又触发案例重审（原有路径，确保未被破坏）。
func TestBodyOnlyClauseChangeInImpactReport(t *testing.T) {
	ia, _ := newImpactAnalyzerFixture(t)
	svc := NewService(ia.gstore)
	astore := ia.astore

	v1, c1 := newPublishedVersionWithClause(t, svc, "准则", "C1", "兼类词按语境标注", "作用域-A")
	createDecidedCaseWithClause(t, astore, v1.ID, 100, 200, []int64{c1.ID})

	v2, _ := newPublishedVersionWithClause(t, svc, "准则", "C1", "兼类词一律按动词标注", "作用域-A")

	report, err := ia.Analyze(v1.ID, v2.ID)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if report.RereviewRequired != 1 {
		t.Fatalf("正文改写应标 1 个重审，得到 %d", report.RereviewRequired)
	}
}

// TestUnchangedClauseNotInImpactReport 对照：正文与作用域均不变时，
// 不产生变更条款，也不标重审（确保修复不引入误报）。
func TestUnchangedClauseNotInImpactReport(t *testing.T) {
	ia, _ := newImpactAnalyzerFixture(t)
	svc := NewService(ia.gstore)
	astore := ia.astore

	v1, c1 := newPublishedVersionWithClause(t, svc, "准则", "C1", "兼类词按语境标注", "作用域-A")
	createDecidedCaseWithClause(t, astore, v1.ID, 100, 200, []int64{c1.ID})

	v2, _ := newPublishedVersionWithClause(t, svc, "准则", "C1", "兼类词按语境标注", "作用域-A")

	report, err := ia.Analyze(v1.ID, v2.ID)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if len(report.ChangedClauses) != 0 {
		t.Fatalf("未变更条款不应出现在 ChangedClauses： %+v", report.ChangedClauses)
	}
	if report.RereviewRequired != 0 {
		t.Fatalf("未变更条款不应触发重审，得到 %d", report.RereviewRequired)
	}
}
