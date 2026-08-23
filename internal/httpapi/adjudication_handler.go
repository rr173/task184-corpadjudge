package httpapi

import (
	"net/http"

	"task184-corpadjudge/internal/model"
)

type createCaseReq struct {
	DisputeID        int64   `json:"dispute_id"`
	GuidelineVersionID int64 `json:"guideline_version_id"`
	ClauseIDs        []int64 `json:"clause_ids"`
}

// handleCreateCase POST /api/cases — 创建裁决案例。
func (s *Server) handleCreateCase(w http.ResponseWriter, r *http.Request) {
	var req createCaseReq
	if err := decode(r, &req); err != nil {
		fail(w, model.ErrInvalidInput)
		return
	}
	c, err := s.app.Adjudication.Create(req.DisputeID, req.GuidelineVersionID, req.ClauseIDs)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, c)
}

// handleListCases GET /api/cases?status=&limit=&offset= — 案例列表。
func (s *Server) handleListCases(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	list, err := s.app.Adjudication.List(status, limit, offset)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, list)
}

// handleGetCase GET /api/cases/{id} — 案例详情。
func (s *Server) handleGetCase(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	c, err := s.app.Adjudication.Get(id)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, c)
}

type decideCaseReq struct {
	Label     string  `json:"label"`
	Rationale string  `json:"rationale"`
	ClauseIDs []int64 `json:"clause_ids"`
}

// handleDecideCase POST /api/cases/{id}/decide — 作出裁决决定。
func (s *Server) handleDecideCase(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	var req decideCaseReq
	if err := decode(r, &req); err != nil {
		fail(w, model.ErrInvalidInput)
		return
	}
	c, err := s.app.Adjudication.Decide(id, req.Label, req.Rationale, req.ClauseIDs)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, c)
}

// handleFormalizeCase POST /api/cases/{id}/formalize — 规则化案例。
func (s *Server) handleFormalizeCase(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	c, err := s.app.Adjudication.Formalize(id)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, c)
}
