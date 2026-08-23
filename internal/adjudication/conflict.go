package adjudication

import (
	"fmt"

	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/store"
)

// ConflictDetector 检测并发裁决时的基准版本冲突。
//
// 语义：裁决案例绑定一个准则版本；若该版本已废止，或存在更新的活动版本，
// 且案例尚未被基准快照收录，则判定为版本冲突，要求裁决者改用新版本重审。
type ConflictDetector struct {
	cases   *store.AdjudicationStore
	guideline GuidelineVersioner
}

// NewConflictDetector 创建冲突检测器。
func NewConflictDetector(c *store.AdjudicationStore, g GuidelineVersioner) *ConflictDetector {
	return &ConflictDetector{cases: c, guideline: g}
}

// Check 检查案例是否可以安全裁决/冻结。
// 返回 nil 表示无冲突；返回 ErrVersionConflict 表示需要重审。
func (cd *ConflictDetector) Check(caseID int64) error {
	c, err := cd.cases.Get(caseID)
	if err != nil {
		return err
	}
	// 已冻结或已规则化的案例不可再直接裁决。
	switch c.Status {
	case model.CaseFormalized, model.CaseSuperseded:
		if c.BaselineID > 0 {
			return fmt.Errorf("%w: case %d frozen in baseline %d", model.ErrFrozen, caseID, c.BaselineID)
		}
	}
	// 引用版本必须仍是活动版本。
	if _, err := cd.guideline.ActiveVersion(c.GuidelineVerID); err != nil {
		return fmt.Errorf("%w: guideline version %d no longer active", model.ErrVersionConflict, c.GuidelineVerID)
	}
	return nil
}

// Snapshotable 报告案例是否满足进入基准快照的条件（已决定/已规则化且未被冻结）。
func (cd *ConflictDetector) Snapshotable(c *model.AdjudicationCase) error {
	switch c.Status {
	case model.CaseDecided, model.CaseFormalized:
		if c.BaselineID > 0 {
			return fmt.Errorf("%w: case %d already in baseline %d", model.ErrDuplicate, c.ID, c.BaselineID)
		}
		return nil
	}
	return fmt.Errorf("%w: case %d in %q cannot enter baseline", model.ErrInvalidState, c.ID, c.Status)
}
