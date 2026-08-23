package httpapi

import (
	"net/http"
	"strconv"

	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/store"
)

// stats 统计各实体数量。
type stats struct {
	GuidelineVersions int `json:"guideline_versions"`
	Clauses           int `json:"clauses"`
	Spans             int `json:"spans"`
	Annotations       int `json:"annotations"`
	Disputes          int `json:"disputes"`
	Cases             int `json:"cases"`
	Baselines         int `json:"baselines"`
	RereviewsOpen     int `json:"rereviews_open"`
}

// handleStats GET /api/stats — 实体统计。
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	st := stats{}
	gv, _ := s.app.Guideline.ListAll()
	st.GuidelineVersions = len(gv)
	for _, v := range gv {
		cs, err := s.app.Guideline.ListClauses(v.ID)
		if err == nil {
			st.Clauses += len(cs)
		}
	}
	spans, _ := s.app.Corpus.List("", 1000, 0)
	st.Spans = len(spans)
	// 标注与分歧、案例通过 SQLite 直接统计。
	st.Annotations = countRows(s.app.DB, "annotations")
	st.Disputes = countRows(s.app.DB, "disputes")
	st.Cases = countRows(s.app.DB, "adjudication_cases")
	st.Baselines = countRows(s.app.DB, "baseline_snapshots")
	st.RereviewsOpen = countRowsWhere(s.app.DB, "rereview_tasks", "status='open'")
	ok(w, st)
}

// handleHealth GET /api/health — 健康检查。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]any{"status": "ok", "service": "task184-corpadjudge"})
}

// handleSelfCheck POST /api/selfcheck — 运行端到端自检（业务闭环 + 幂等 + 指纹去重）。
func (s *Server) handleSelfCheck(w http.ResponseWriter, r *http.Request) {
	res := s.runSelfCheck()
	if !res.OK {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"data": res})
		return
	}
	ok(w, res)
}

// SelfCheckResult 自检输出。
type SelfCheckResult struct {
	OK                 bool              `json:"ok"`
	Steps              []string          `json:"steps"`
	DisputeCreated     bool              `json:"dispute_created"`
	IdempotentSubmit   bool              `json:"idempotent_submit"`
	FingerprintDedup   bool              `json:"fingerprint_dedup"`
	BaselineFrozen     bool              `json:"baseline_frozen"`
	ImpactDetected     bool              `json:"impact_detected"`
	RuleCompiled       bool              `json:"rule_compiled"`
}

// runSelfCheck 执行完整业务闭环，验证核心不变量。
func (s *Server) runSelfCheck() SelfCheckResult {
	res := SelfCheckResult{OK: true, Steps: []string{}}
	app := s.app

	// 1. 准则版本与条款。
	v, err := app.Guideline.CreateVersion("自检准则", "自检用准则版本", "selfcheck")
	if err != nil {
		res.OK = false
		res.Steps = append(res.Steps, "创建准则版本失败: "+err.Error())
		return res
	}
	clause, err := app.Guideline.AddClause(v.ID, "C1", model.LayerCoref, "指代边界", "边界应落在名词短语边界", "核心指代")
	if err != nil {
		res.OK = false
		res.Steps = append(res.Steps, "添加条款失败: "+err.Error())
		return res
	}
	if _, err := app.Guideline.Publish(v.ID); err != nil {
		res.OK = false
		res.Steps = append(res.Steps, "发布准则失败: "+err.Error())
		return res
	}
	res.Steps = append(res.Steps, "准则版本发布 + 条款 C1")

	// 2. 片段创建 + 指纹幂等。
	sp, err := app.Corpus.Create("doc1", 0, 12, "张三把文件给了李四", model.LayerCoref)
	if err != nil {
		res.OK = false
		res.Steps = append(res.Steps, "创建片段失败: "+err.Error())
		return res
	}
	sp2, err := app.Corpus.Create("doc1", 0, 12, "张三把文件给了李四", model.LayerCoref)
	if err != nil || sp2.ID != sp.ID {
		res.OK = false
		res.Steps = append(res.Steps, "片段指纹幂等失败")
		return res
	}
	res.FingerprintDedup = true
	res.Steps = append(res.Steps, "片段创建 + 指纹去重")

	// 3. 三位标注员提交两种标签 → 分歧。
	labels := []string{"B-LOC", "B-PER", "B-PER"}
	annotators := []string{"ann1", "ann2", "ann3"}
	for i := 0; i < 3; i++ {
		_, merge, err := app.Workflow(sp.ID, annotators[i], model.LayerCoref, labels[i], 0, 2, v.ID)
		if err != nil {
			res.OK = false
			res.Steps = append(res.Steps, "标注提交失败: "+err.Error())
			return res
		}
		_ = merge
	}
	// 幂等验证：ann1 重复提交相同标签只产生一条记录。
	_, _, _ = app.Workflow(sp.ID, "ann1", model.LayerCoref, "B-LOC", 0, 2, v.ID)
	res.IdempotentSubmit = true
	anns, _ := app.Annotation.ListBySpan(sp.ID, string(model.LayerCoref))
	count := 0
	for _, a := range anns {
		if a.Annotator == "ann1" {
			count++
		}
	}
	if count != 1 {
		res.OK = false
		res.Steps = append(res.Steps, "幂等投票失败：ann1 存在多条记录")
		return res
	}
	res.Steps = append(res.Steps, "三人标注 → 分歧 + 幂等验证")

	// 4. 查询分歧与矩阵。
	disputes, _ := app.Dispute.List(sp.ID, 10, 0)
	if len(disputes) == 0 {
		res.OK = false
		res.Steps = append(res.Steps, "未检测到分歧")
		return res
	}
	res.DisputeCreated = true
	matrix, err := app.Dispute.Matrix(disputes[0].ID)
	if err != nil || len(matrix) < 2 {
		res.OK = false
		res.Steps = append(res.Steps, "分歧矩阵计算失败")
		return res
	}
	res.Steps = append(res.Steps, "分歧矩阵（2 个标签）")

	// 5. 裁决 + 规则化 + 冻结。
	_, snap, err := app.AdjudicateAndFreeze(disputes[0].ID, v.ID, "B-PER", "名词短语边界，引用 C1",
		[]int64{clause.ID}, "自检基准-1")
	if err != nil {
		res.OK = false
		res.Steps = append(res.Steps, "裁决/冻结失败: "+err.Error())
		return res
	}
	if !snap.Frozen {
		res.OK = false
		res.Steps = append(res.Steps, "基准快照未冻结")
		return res
	}
	res.BaselineFrozen = true
	res.Steps = append(res.Steps, "裁决 → 规则化 → 基准冻结")

	// 6. 准则更新 → 影响分析。
	v2, err := app.Guideline.CreateVersion("自检准则", "更新后的准则", "selfcheck v2")
	if err != nil {
		res.OK = false
		res.Steps = append(res.Steps, "创建新准则版本失败: "+err.Error())
		return res
	}
	if _, err := app.Guideline.AddClause(v2.ID, "C1", model.LayerCoref, "指代边界", "边界应落在名词短语边界且仅允许专有名词", "核心指代"); err != nil {
		res.OK = false
		res.Steps = append(res.Steps, "添加新条款失败: "+err.Error())
		return res
	}
	if _, err := app.Guideline.Publish(v2.ID); err != nil {
		res.OK = false
		res.Steps = append(res.Steps, "发布新准则失败: "+err.Error())
		return res
	}
	report, err := app.Impact.Analyze(v.ID, v2.ID)
	if err != nil {
		res.OK = false
		res.Steps = append(res.Steps, "影响分析失败: "+err.Error())
		return res
	}
	if report.RereviewRequired == 0 {
		res.OK = false
		res.Steps = append(res.Steps, "影响分析未标出重审案例")
		return res
	}
	res.ImpactDetected = true
	res.Steps = append(res.Steps, "准则更新 → 影响分析标出 "+strconv.Itoa(report.RereviewRequired)+" 个重审案例")

	// 8. 规则编译检查：裁决案例应已编译为可回放规则。
	cases, err := app.Adjudication.List(model.CaseFormalized, 10, 0)
	if err != nil || len(cases) == 0 {
		res.OK = false
		res.Steps = append(res.Steps, "规则化案例缺失")
		return res
	}
	if cases[0].FormalizedRule == "" {
		res.OK = false
		res.Steps = append(res.Steps, "规则未编译")
		return res
	}
	res.RuleCompiled = true
	res.Steps = append(res.Steps, "规则编译: "+cases[0].FormalizedRule)
	return res
}

func countRows(db *store.DB, table string) int {
	var n int
	_ = db.SQL().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n)
	return n
}

func countRowsWhere(db *store.DB, table, where string) int {
	var n int
	_ = db.SQL().QueryRow("SELECT COUNT(*) FROM " + table + " WHERE " + where).Scan(&n)
	return n
}
