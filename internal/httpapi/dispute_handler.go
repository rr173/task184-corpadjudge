package httpapi

import (
	"net/http"

	"task184-corpadjudge/internal/dispute"
	"task184-corpadjudge/internal/model"
)

// handleListDisputes GET /api/disputes?span_id=&limit=&offset= — 分歧列表。
func (s *Server) handleListDisputes(w http.ResponseWriter, r *http.Request) {
	spanID := queryInt(r, "span_id", 0)
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	list, err := s.app.Dispute.List(int64(spanID), limit, offset)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, list)
}

// handleGetDispute GET /api/disputes/{id} — 分歧详情（含矩阵）。
func (s *Server) handleGetDispute(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	d, err := s.app.Dispute.Get(id)
	if err != nil {
		fail(w, err)
		return
	}
	matrix, err := s.app.Dispute.Matrix(id)
	if err != nil {
		fail(w, err)
		return
	}
	// 稳定排序：票数降序 + 并列按标签字母升序，并列标签顺序与首位一致。
	dispute.RankMatrixEntries(matrix)
	ok(w, map[string]any{"dispute": d, "matrix": matrix})
}

// handleMatrix POST /api/disputes/{id}/matrix — 计算分歧矩阵。
func (s *Server) handleMatrix(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	matrix, err := s.app.Dispute.Matrix(id)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, matrix)
}

type refClausesReq struct {
	Label     string  `json:"label"`
	ClauseIDs []int64 `json:"clause_ids"`
}

// handleReferenceClauses POST /api/disputes/{id}/refs — 为矩阵条目补充条款引用。
func (s *Server) handleReferenceClauses(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	var req refClausesReq
	if err := decode(r, &req); err != nil {
		fail(w, model.ErrInvalidInput)
		return
	}
	if err := s.app.Dispute.ReferenceClauses(id, req.Label, req.ClauseIDs); err != nil {
		fail(w, err)
		return
	}
	matrix, err := s.app.Dispute.Matrix(id)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, matrix)
}
