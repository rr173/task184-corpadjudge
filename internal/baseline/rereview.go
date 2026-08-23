package baseline

import (
	"fmt"

	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/store"
)

// Rereview 是重审任务管理：准则更新后，旧基准案例被标记待重审。
type Rereview struct {
	bstore *store.BaselineStore
	astore *store.AdjudicationStore
}

// NewRereview 创建重审管理器。
func NewRereview(b *store.BaselineStore, a *store.AdjudicationStore) *Rereview {
	return &Rereview{bstore: b, astore: a}
}

// Create 为受影响案例创建重审任务（幂等：同 case+from+to 已存在则跳过）。
func (r *Rereview) Create(caseID, fromVersionID, toVersionID int64, reason string) (*model.RereviewTask, error) {
	if reason == "" {
		reason = "guideline update"
	}
	existing, err := r.bstore.ListRereviews("open", 500, 0)
	if err != nil {
		return nil, err
	}
	for _, e := range existing {
		if e.CaseID == caseID && e.FromVersionID == fromVersionID && e.ToVersionID == toVersionID {
			return &e, nil // 幂等返回既有任务
		}
	}
	t := &model.RereviewTask{
		CaseID: caseID, FromVersionID: fromVersionID, ToVersionID: toVersionID,
		Reason: reason, Status: "open", CreatedAt: store.Now(),
	}
	// 关联基准：若案例已入基准，则取该基准 ID。
	if c, err := r.astore.Get(caseID); err == nil && c.BaselineID > 0 {
		t.BaselineID = c.BaselineID
	}
	id, err := r.bstore.CreateRereview(t)
	if err != nil {
		return nil, err
	}
	t.ID = id
	return t, nil
}

// List 列出重审任务。
func (r *Rereview) List(status string, limit, offset int) ([]model.RereviewTask, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return r.bstore.ListRereviews(status, limit, offset)
}

// Resolve 解决重审任务：记录结论，若决定改变则标记原案例已替代。
func (r *Rereview) Resolve(id int64, note string, newLabel string, newCaseID int64) (*model.RereviewTask, error) {
	tasks, err := r.bstore.ListRereviews("open", 1000, 0)
	if err != nil {
		return nil, err
	}
	var task *model.RereviewTask
	for i := range tasks {
		if tasks[i].ID == id {
			task = &tasks[i]
			break
		}
	}
	if task == nil {
		return nil, fmt.Errorf("%w: open rereview task %d not found", model.ErrNotFound, id)
	}
	if newLabel != "" && newCaseID > 0 {
		// 新决定已生成：标记旧案例已替代。
		if err := r.astore.MarkSuperseded(task.CaseID); err != nil {
			return nil, err
		}
	}
	if err := r.bstore.ResolveRereview(id, note, store.Now()); err != nil {
		return nil, err
	}
	// 返回解决后的任务。
	resolved, err := r.bstore.ListRereviews("resolved", 1000, 0)
	if err != nil {
		return nil, err
	}
	for i := range resolved {
		if resolved[i].ID == id {
			return &resolved[i], nil
		}
	}
	return nil, fmt.Errorf("%w: rereview task %d not found after resolve", model.ErrNotFound, id)
}
