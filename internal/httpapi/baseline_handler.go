package httpapi

import (
	"net/http"

	"task184-corpadjudge/internal/model"
)

type createSnapshotReq struct {
	Name               string `json:"name"`
	GuidelineVersionID int64  `json:"guideline_version_id"`
}

// handleCreateSnapshot POST /api/baselines — 创建基准快照（草稿）。
func (s *Server) handleCreateSnapshot(w http.ResponseWriter, r *http.Request) {
	var req createSnapshotReq
	if err := decode(r, &req); err != nil {
		fail(w, model.ErrInvalidInput)
		return
	}
	snap, err := s.app.Baseline.CreateSnapshot(req.Name, req.GuidelineVersionID)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, snap)
}

// handleListSnapshots GET /api/baselines — 基准快照列表。
func (s *Server) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	list, err := s.app.Baseline.List()
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, list)
}

// handleGetSnapshot GET /api/baselines/{id} — 快照详情（含案例）。
func (s *Server) handleGetSnapshot(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	snap, cases, err := s.app.Baseline.Get(id)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]any{"snapshot": snap, "cases": cases})
}

type freezeSnapshotReq struct {
	CaseIDs []int64 `json:"case_ids"`
}

// handleFreezeSnapshot POST /api/baselines/{id}/freeze — 冻结快照。
func (s *Server) handleFreezeSnapshot(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	var req freezeSnapshotReq
	if err := decode(r, &req); err != nil {
		fail(w, model.ErrInvalidInput)
		return
	}
	snap, err := s.app.Baseline.Freeze(id, req.CaseIDs)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, snap)
}

// handleExportSnapshot GET /api/baselines/{id}/export — 导出快照（JSON）。
func (s *Server) handleExportSnapshot(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	out, err := s.app.Baseline.Export(id)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, out)
}

type resolveRereviewReq struct {
	Note      string `json:"note"`
	NewLabel  string `json:"new_label"`
	NewCaseID int64  `json:"new_case_id"`
}

// handleListRereviews GET /api/rereviews?status=&limit=&offset= — 重审任务列表。
func (s *Server) handleListRereviews(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	list, err := s.app.Rereview.List(status, limit, offset)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, list)
}

// handleResolveRereview POST /api/rereviews/{id}/resolve — 处理重审任务。
func (s *Server) handleResolveRereview(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	var req resolveRereviewReq
	if err := decode(r, &req); err != nil {
		fail(w, model.ErrInvalidInput)
		return
	}
	t, err := s.app.Rereview.Resolve(id, req.Note, req.NewLabel, req.NewCaseID)
	if err != nil {
		// 重审结果由服务层关联到新案例。
		fail(w, err)
		return
	}
	ok(w, t)
}
