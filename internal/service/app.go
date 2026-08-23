// Package service 编排各业务包，提供端到端业务闭环与自检。
//
// 完整闭环：
//  1. 负责人发布准则版本与条款（guideline）
//  2. 语料片段入库（corpus，指纹幂等）
//  3. 标注员提交标注（annotation，vote_key 幂等 + 层级混淆防护）
//  4. 系统按片段归并：一致 or 分歧（corpus.Merger）
//  5. 裁决者引用条款裁决（adjudication），案例入基准（baseline）
//  6. 准则更新触发影响分析与重审任务（guideline.ImpactAnalyzer + baseline.Rereview）
package service

import (
	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/store"

	"task184-corpadjudge/internal/adjudication"
	"task184-corpadjudge/internal/annotation"
	"task184-corpadjudge/internal/baseline"
	"task184-corpadjudge/internal/corpus"
	"task184-corpadjudge/internal/dispute"
	"task184-corpadjudge/internal/guideline"
)

// App 聚合全部业务服务的门面。
type App struct {
	DB *store.DB

	Guideline    *guideline.Service
	Corpus       *corpus.Service
	Merger       *corpus.Merger
	Annotation   *annotation.Service
	CrossLayer   *annotation.CrossLayerGuard
	Dispute      *dispute.Service
	Adjudication *adjudication.Service
	Conflict     *adjudication.ConflictDetector
	Baseline     *baseline.Service
	Rereview     *baseline.Rereview
	Impact       *guideline.ImpactAnalyzer
}

// New 组装全部服务。
func New(db *store.DB) (*App, error) {
	gstore := store.NewGuidelineStore(db)
	cstore := store.NewCorpusStore(db)
	astore := store.NewAnnotationStore(db)
	dstore := store.NewDisputeStore(db)
	acstore := store.NewAdjudicationStore(db)
	bstore := store.NewBaselineStore(db)

	gsvc := guideline.NewService(gstore)
	csvc := corpus.NewService(cstore)
	asvc := annotation.NewService(astore, cstore)
	dsvc := dispute.NewService(dstore, astore)
	adjsvc := adjudication.NewService(acstore, dstore, cstore, gsvc)
	bssvc := baseline.NewService(bstore, acstore, cstore)
	rrsvc := baseline.NewRereview(bstore, acstore)

	return &App{
		DB:           db,
		Guideline:    gsvc,
		Corpus:       csvc,
		Merger:       corpus.NewMerger(cstore, astore, dstore),
		Annotation:   asvc,
		CrossLayer:   annotation.NewCrossLayerGuard(astore),
		Dispute:      dsvc,
		Adjudication: adjsvc,
		Conflict:     adjudication.NewConflictDetector(acstore, gsvc),
		Baseline:     bssvc,
		Rereview:     rrsvc,
		Impact:       guideline.NewImpactAnalyzer(gstore, acstore, bstore),
	}, nil
}

// Workflow 提交标注并触发归并：创建/幂等草稿 → 提交 → 归并片段。
// 返回标注与归并结果；归并产生分歧时返回 DisputeID。
func (a *App) Workflow(spanID int64, annotator string, layer model.Layer, label string,
	start, end int, versionID int64) (*model.Annotation, *corpus.MergeResult, error) {
	// 层级混淆防护。
	if err := a.CrossLayer.Check(annotator, spanID, layer); err != nil {
		return nil, nil, err
	}
	ann, err := a.Annotation.Draft(spanID, annotator, layer, label, start, end, versionID)
	if err != nil {
		return nil, nil, err
	}
	if ann.Status != model.AnnSubmitted {
		if _, err := a.Annotation.Submit(ann.ID); err != nil {
			return nil, nil, err
		}
	}
	merge, err := a.Merger.MergeSpan(spanID, layer)
	if err != nil {
		return nil, nil, err
	}
	return ann, merge, nil
}

// AdjudicateAndFreeze 端到端裁决并冻结：
// 创建案例 → 冲突检测 → 决定 → 规则化 → 创建基准快照 → 冻结片段。
func (a *App) AdjudicateAndFreeze(disputeID, guidelineVerID int64, label, rationale string,
	clauseIDs []int64, snapshotName string) (*model.AdjudicationCase, *model.BaselineSnapshot, error) {
	c, err := a.Adjudication.Create(disputeID, guidelineVerID, clauseIDs)
	if err != nil {
		return nil, nil, err
	}
	if err := a.Conflict.Check(c.ID); err != nil {
		return nil, nil, err
	}
	if _, err := a.Adjudication.Decide(c.ID, label, rationale, clauseIDs); err != nil {
		return nil, nil, err
	}
	if _, err := a.Adjudication.Formalize(c.ID); err != nil {
		return nil, nil, err
	}
	// 重读以获取 formalized 状态与编译后的规则。
	final, err := a.Adjudication.Get(c.ID)
	if err != nil {
		return nil, nil, err
	}
	snap, err := a.Baseline.CreateSnapshot(snapshotName, guidelineVerID)
	if err != nil {
		return nil, nil, err
	}
	frozen, err := a.Baseline.Freeze(snap.ID, []int64{c.ID})
	if err != nil {
		return nil, nil, err
	}
	// 片段状态推进：分歧 → 已裁决 → 冻结。
	if err := a.Corpus.MarkStatus(final.SpanID, model.SpanAdjudicated); err != nil {
		return nil, nil, err
	}
	if _, err := a.Corpus.Freeze(final.SpanID); err != nil {
		return nil, nil, err
	}
	return final, frozen, nil
}

// SelfCheckResult 自检结果：统计 + 幂等验证 + 持久化验证。
type SelfCheckResult struct {
	GuidelineVersions  int            `json:"guideline_versions"`
	Spans              int            `json:"spans"`
	Annotations        int            `json:"annotations"`
	Disputes           int            `json:"disputes"`
	Cases              int            `json:"cases"`
	Baselines          int            `json:"baselines"`
	RereviewsOpen      int            `json:"rereviews_open"`
	Idempotency        map[string]any `json:"idempotency_checks"`
	FingerprintDedup   bool           `json:"fingerprint_dedup"`
	PersistenceRestore bool           `json:"persistence_restore"`
}
