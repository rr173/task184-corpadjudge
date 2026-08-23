// Package model 定义语言语料标注准则分歧裁决台的领域实体与统一错误。
package model

import (
	"errors"
	"fmt"
)

// 领域错误。所有业务包与 store 层均返回这些哨兵错误，httpapi 层将其映射为 HTTP 状态码。
var (
	// ErrNotFound 表示目标资源不存在。
	ErrNotFound = errors.New("resource not found")
	// ErrInvalidState 表示在非法状态下执行了状态转换（状态机守卫失败）。
	ErrInvalidState = errors.New("invalid state transition")
	// ErrDuplicate 表示唯一约束冲突（片段指纹重复、标注员重复投票等）。
	ErrDuplicate = errors.New("duplicate resource")
	// ErrInvalidInput 表示入参校验失败（越界标签、边界非法、层级混淆等）。
	ErrInvalidInput = errors.New("invalid input")
	// ErrFrozen 表示对已冻结对象执行了直接改写。
	ErrFrozen = errors.New("object is frozen")
	// ErrVersionConflict 表示并发裁决/冻结时的基准版本冲突。
	ErrVersionConflict = errors.New("baseline version conflict")
	// ErrStaleGuideline 表示引用了已废止或非活动准则版本。
	ErrStaleGuideline = errors.New("guideline version not active")
)

// 各实体的状态常量。
const (
	// GuidelineVersion 状态。
	GuidelineDraft     = "draft"     // 草拟
	GuidelinePublished = "published" // 已发布
	GuidelineAmbiguous = "ambiguous" // 存在歧义
	GuidelineRevoked   = "revoked"   // 已废止

	// CorpusSpan 状态。
	SpanPending     = "pending"     // 待标注
	SpanConsistent  = "consistent"  // 一致
	SpanDisputed    = "disputed"    // 存在分歧
	SpanAdjudicated = "adjudicated" // 已裁决
	SpanFrozen      = "frozen"      // 已冻结

	// Annotation 状态。
	AnnDraft    = "draft"    // 草稿
	AnnSubmitted = "submitted" // 已提交
	AnnAccepted = "accepted" // 被采纳
	AnnRejected = "rejected" // 被驳回
	AnnRereview = "rereview" // 待重审

	// AdjudicationCase 状态。
	CasePending    = "pending"    // 待裁决
	CaseDecided    = "decided"    // 已决定
	CaseFormalized = "formalized" // 规则化
	CaseSuperseded = "superseded" // 已替代
)

// Layer 表示标注标签层（词性 / 指代 / 语义角色）。
type Layer string

const (
	LayerPOS       Layer = "pos"       // 词性标注
	LayerCoref     Layer = "coref"     // 指代标注
	LayerSemRole   Layer = "semrole"   // 语义角色标注
	LayerNamedEnt  Layer = "namedent"  // 命名实体
	LayerSentiment Layer = "sentiment" // 情感极性
)

// ValidLayer 报告 layer 是否为受支持的标签层。
func ValidLayer(l Layer) bool {
	switch l {
	case LayerPOS, LayerCoref, LayerSemRole, LayerNamedEnt, LayerSentiment:
		return true
	}
	return false
}

// AllLayers 返回全部受支持标签层。
func AllLayers() []Layer {
	return []Layer{LayerPOS, LayerCoref, LayerSemRole, LayerNamedEnt, LayerSentiment}
}

// GuidelineVersion 是准则版本，承载一组条款。
type GuidelineVersion struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	VersionNo    int    `json:"version_no"`
	CreatedAt    string `json:"created_at"`
	PublishedAt  string `json:"published_at,omitempty"`
	AmbiguousAt  string `json:"ambiguous_at,omitempty"`
	RevokedAt    string `json:"revoked_at,omitempty"`
	RevisionNote string `json:"revision_note,omitempty"`
}

// GuidelineClause 是准则版本中的一条条款，声明某一标签层的作用域与判定规则。
type GuidelineClause struct {
	ID        int64  `json:"id"`
	VersionID int64  `json:"version_id"`
	ClauseNo  string `json:"clause_no"`
	Layer     Layer  `json:"layer"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Scope     string `json:"scope"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"created_at"`
}

// CorpusSpan 是语料片段：文档中一段连续文本，按 (doc_id, start, end, text) 计算指纹。
type CorpusSpan struct {
	ID          int64  `json:"id"`
	DocID       string `json:"doc_id"`
	StartOffset int    `json:"start_offset"`
	EndOffset   int    `json:"end_offset"`
	Text        string `json:"text"`
	Fingerprint string `json:"fingerprint"`
	Layer       Layer  `json:"layer"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	FrozenAt    string `json:"frozen_at,omitempty"`
}

// Annotation 是标注员对某片段某层提交的一条标签投票。
type Annotation struct {
	ID             int64  `json:"id"`
	SpanID         int64  `json:"span_id"`
	Annotator      string `json:"annotator"`
	Layer          Layer  `json:"layer"`
	Label          string `json:"label"`
	StartOffset    int    `json:"start_offset"`
	EndOffset      int    `json:"end_offset"`
	GuidelineVerID int64  `json:"guideline_version_id"`
	Status         string `json:"status"`
	VoteKey        string `json:"vote_key"` // annotator:span:layer:label:version 幂等键
	SubmittedAt    string `json:"submitted_at,omitempty"`
	DecidedAt      string `json:"decided_at,omitempty"`
}

// Dispute 是按片段归并出的分歧记录，绑定一个分歧矩阵。
type Dispute struct {
	ID         int64  `json:"id"`
	SpanID     int64  `json:"span_id"`
	Layer      Layer  `json:"layer"`
	Status     string `json:"status"`
	LabelCount int    `json:"label_count"`
	CreatedAt  string `json:"created_at"`
	ResolvedAt string `json:"resolved_at,omitempty"`
}

// MatrixEntry 是分歧矩阵中的一行：某标签 + 投票人 + 引用的条款。
type MatrixEntry struct {
	Label       string   `json:"label"`
	Annotators  []string `json:"annotators"`
	ClauseRefs  []int64  `json:"clause_refs"`
	VoteCount   int      `json:"vote_count"`
}

// AdjudicationCase 是裁决案例：针对一个分歧，引用条款后作出决定。
type AdjudicationCase struct {
	ID             int64   `json:"id"`
	DisputeID      int64   `json:"dispute_id"`
	SpanID         int64   `json:"span_id"`
	Status         string  `json:"status"`
	DecidedLabel   string  `json:"decided_label,omitempty"`
	Rationale      string  `json:"rationale,omitempty"`
	ClauseIDs      []int64 `json:"clause_ids"`
	GuidelineVerID int64   `json:"guideline_version_id"`
	BaselineID     int64   `json:"baseline_id,omitempty"`
	CreatedAt      string  `json:"created_at"`
	DecidedAt      string  `json:"decided_at,omitempty"`
	FormalizedRule string  `json:"formalized_rule,omitempty"`
}

// BaselineSnapshot 是冻结的基准集快照：绑定准则版本，收录一组裁决案例。
type BaselineSnapshot struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	GuidelineVerID    int64  `json:"guideline_version_id"`
	CaseCount         int    `json:"case_count"`
	Frozen            bool   `json:"frozen"`
	CreatedAt         string `json:"created_at"`
	FrozenAt          string `json:"frozen_at,omitempty"`
}

// BaselineCase 是基准快照内的案例条目，绑定原片段、标签与条款（不可变）。
type BaselineCase struct {
	ID           int64  `json:"id"`
	SnapshotID   int64  `json:"snapshot_id"`
	CaseID       int64  `json:"case_id"`
	SpanID       int64  `json:"span_id"`
	SpanFingerprint string `json:"span_fingerprint"`
	DecidedLabel string `json:"decided_label"`
	ClauseID     int64  `json:"clause_id"`
	GuidelineVerID int64 `json:"guideline_version_id"`
	FrozenAt     string `json:"frozen_at"`
}

// RereviewTask 是准则更新后系统标出的重审任务：旧基准案例需按新准则复核。
type RereviewTask struct {
	ID             int64  `json:"id"`
	CaseID         int64  `json:"case_id"`
	BaselineID     int64  `json:"baseline_id"`
	FromVersionID  int64  `json:"from_version_id"`
	ToVersionID    int64  `json:"to_version_id"`
	Reason         string `json:"reason"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	ResolvedAt     string `json:"resolved_at,omitempty"`
	ResolutionNote string `json:"resolution_note,omitempty"`
}

// Wrap 包装底层错误并附带上下文。
func Wrap(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), err)
}
