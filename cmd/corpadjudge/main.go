// Command corpadjudge 是语言语料标注准则分歧裁决台的入口。
//
// 支持三个标志：
//   - --addr :8080      监听地址（默认 :8080）
//   - --db ./data.db    SQLite 数据库路径（默认 ./corpadjudge.db）
//   - --smoke-test      执行端到端冒烟：真实创建数据、关闭并重开数据库
//                       验证持久化与重启恢复，随后以 0 退出码结束。
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"task184-corpadjudge/internal/httpapi"
	"task184-corpadjudge/internal/service"
	"task184-corpadjudge/internal/store"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dbPath := flag.String("db", "./corpadjudge.db", "SQLite database path")
	smoke := flag.Bool("smoke-test", false, "run end-to-end smoke test and exit")
	flag.Parse()

	if *smoke {
		if err := runSmokeTest(*dbPath); err != nil {
			fmt.Fprintln(os.Stderr, "SMOKE TEST FAILED:", err)
			os.Exit(1)
		}
		fmt.Println("SMOKE TEST PASSED")
		return
	}

	db, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	app, err := service.New(db)
	if err != nil {
		log.Fatalf("init services: %v", err)
	}
	srv := httpapi.New(app)
	log.Printf("task184-corpadjudge listening on %s (db=%s)", *addr, *dbPath)
	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		log.Fatalf("http server: %v", err)
	}
}

// runSmokeTest 执行冒烟测试：
//  1. 打开数据库 A，完整跑一遍业务闭环（准则→片段→标注→分歧→裁决→冻结→影响分析）
//  2. 幂等验证：重复提交同一标注与重复创建同一片段均不产生重复数据
//  3. 关闭数据库 A，重新打开同一路径数据库 B，验证数据仍在（重启恢复）
//  4. 校验断点语义：冻结案例绑定原片段指纹与原准则版本，不受准则更新影响
func runSmokeTest(dbPath string) error {
	if dbPath != ":memory:" {
		_ = os.Remove(dbPath)
	}

	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	app, err := service.New(db)
	if err != nil {
		db.Close()
		return fmt.Errorf("init services: %w", err)
	}

	// --- 步骤 1：业务闭环 ---
	v1, err := app.Guideline.CreateVersion("词性标注准则", "面向中文语料的词性标注规范", "v1")
	if err != nil {
		db.Close()
		return fmt.Errorf("create guideline: %w", err)
	}
	c1, err := app.Guideline.AddClause(v1.ID, "G-POS-1", "pos", "词性判定", "兼类词按语境词性标注", "动词/名词兼类")
	if err != nil {
		db.Close()
		return fmt.Errorf("add clause: %w", err)
	}
	v1, err = app.Guideline.Publish(v1.ID)
	if err != nil {
		db.Close()
		return fmt.Errorf("publish guideline: %w", err)
	}

	sp, err := app.Corpus.Create("doc-001", 0, 8, "小明喜欢学习编程", "pos")
	if err != nil {
		db.Close()
		return fmt.Errorf("create span: %w", err)
	}
	// 指纹幂等：重复创建同片段返回既有 ID。
	spDup, err := app.Corpus.Create("doc-001", 0, 8, "小明喜欢学习编程", "pos")
	if err != nil || spDup.ID != sp.ID {
		db.Close()
		return fmt.Errorf("span fingerprint dedup failed")
	}

	// 三位标注员：两种标签 → 分歧。
	votes := []struct {
		annotator string
		label     string
	}{
		{"ann-a", "V"}, {"ann-b", "N"}, {"ann-c", "V"},
	}
	for _, v := range votes {
		_, _, err := app.Workflow(sp.ID, v.annotator, "pos", v.label, 4, 6, v1.ID)
		if err != nil {
			db.Close()
			return fmt.Errorf("workflow submit %s: %w", v.annotator, err)
		}
	}
	// 幂等投票：ann-a 重复提交相同标签 → 仍只有一条记录。
	if _, _, err := app.Workflow(sp.ID, "ann-a", "pos", "V", 4, 6, v1.ID); err != nil {
		db.Close()
		return fmt.Errorf("idempotent resubmit: %w", err)
	}

	disputes, err := app.Dispute.List(sp.ID, 10, 0)
	if err != nil || len(disputes) == 0 {
		db.Close()
		return fmt.Errorf("dispute not created (err=%v, n=%d)", err, len(disputes))
	}
	matrix, err := app.Dispute.Matrix(disputes[0].ID)
	if err != nil || len(matrix) != 2 {
		db.Close()
		return fmt.Errorf("matrix expected 2 labels, got %d (err=%v)", len(matrix), err)
	}

	// 裁决 + 规则化 + 冻结。
	decided, snap, err := app.AdjudicateAndFreeze(disputes[0].ID, v1.ID, "V", "语境为动词用法，引用 G-POS-1",
		[]int64{c1.ID}, "基准-001")
	if err != nil {
		db.Close()
		return fmt.Errorf("adjudicate+freeze: %w", err)
	}
	if decided.FormalizedRule == "" || !snap.Frozen {
		db.Close()
		return fmt.Errorf("case not formalized or snapshot not frozen")
	}

	// 准则更新 → 影响分析。
	v2, err := app.Guideline.CreateVersion("词性标注准则", "更新后的词性标注规范", "v2")
	if err != nil {
		db.Close()
		return fmt.Errorf("create v2: %w", err)
	}
	if _, err := app.Guideline.AddClause(v2.ID, "G-POS-1", "pos", "词性判定", "兼类词一律按动词标注", "动词/名词兼类"); err != nil {
		db.Close()
		return fmt.Errorf("add v2 clause: %w", err)
	}
	v2, err = app.Guideline.Publish(v2.ID)
	if err != nil {
		db.Close()
		return fmt.Errorf("publish v2: %w", err)
	}
	report, err := app.Impact.Analyze(v1.ID, v2.ID)
	if err != nil {
		db.Close()
		return fmt.Errorf("impact analyze: %w", err)
	}
	if report.RereviewRequired != 1 {
		db.Close()
		return fmt.Errorf("expected 1 rereview, got %d", report.RereviewRequired)
	}

	// --- 步骤 2：关闭并重开，验证持久化恢复 ---
	if err := db.Close(); err != nil {
		return fmt.Errorf("close db: %w", err)
	}
	db2, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("reopen db: %w", err)
	}
	defer db2.Close()
	app2, err := service.New(db2)
	if err != nil {
		return fmt.Errorf("re-init services: %w", err)
	}

	// 数据仍在。
	gotV1, err := app2.Guideline.Get(v1.ID)
	if err != nil || gotV1.Status != "published" {
		return fmt.Errorf("restore guideline failed (err=%v)", err)
	}
	gotSpan, err := app2.Corpus.Get(sp.ID)
	if err != nil || gotSpan.Status != "frozen" {
		return fmt.Errorf("restore span failed (err=%v, status=%q)", err, gotSpan.Status)
	}
	gotCase, err := app2.Adjudication.Get(decided.ID)
	if err != nil || gotCase.FormalizedRule == "" {
		return fmt.Errorf("restore case failed (err=%v)", err)
	}
	snaps, err := app2.Baseline.List()
	if err != nil || len(snaps) == 0 || !snaps[0].Frozen {
		return fmt.Errorf("restore baseline failed (err=%v, n=%d)", err, len(snaps))
	}
	// 旧基准保留原决定：案例仍绑定 v1 与片段指纹。
	_, cases, err := app2.Baseline.Get(snaps[0].ID)
	if err != nil || len(cases) == 0 {
		return fmt.Errorf("baseline cases missing after restore")
	}
	if cases[0].GuidelineVerID != v1.ID {
		return fmt.Errorf("baseline case not bound to original guideline version")
	}
	// 冻结案例不可直接改写：重新裁决应触发冲突/冻结守卫。
	if err := app2.Conflict.Check(gotCase.ID); err == nil {
		return fmt.Errorf("expected conflict for frozen case")
	}

	// 重审任务创建（模拟人工重审入口）。
	task, err := app2.Rereview.Create(decided.ID, v1.ID, v2.ID, "条款 G-POS-1 已改写")
	if err != nil {
		return fmt.Errorf("create rereview: %w", err)
	}
	if _, err := app2.Rereview.Resolve(task.ID, "新准则确认仍为动词", "V", 0); err != nil {
		return fmt.Errorf("resolve rereview: %w", err)
	}
	open, err := app2.Rereview.List("open", 10, 0)
	if err != nil || len(open) != 0 {
		return fmt.Errorf("rereview queue not empty after resolve (n=%d)", len(open))
	}

	fmt.Printf("smoke: guideline=%d(%s) span=%d fingerprint=%q dispute=%d case=%d baseline=%d rereviews=0\n",
		v1.ID, v1.Status, sp.ID, sp.Fingerprint, disputes[0].ID, decided.ID, snap.ID)
	return nil
}
