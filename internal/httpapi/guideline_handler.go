package httpapi

import (
	"net/http"

	"task184-corpadjudge/internal/model"
)

type createGuidelineReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Note        string `json:"note"`
}

type clauseReq struct {
	ClauseNo string    `json:"clause_no"`
	Layer    model.Layer `json:"layer"`
	Title    string    `json:"title"`
	Body     string    `json:"body"`
	Scope    string    `json:"scope"`
}

// handleCreateGuideline POST /api/guidelines — 创建草拟准则版本。
func (s *Server) handleCreateGuideline(w http.ResponseWriter, r *http.Request) {
	var req createGuidelineReq
	if err := decode(r, &req); err != nil {
		fail(w, model.ErrInvalidInput)
		return
	}
	v, err := s.app.Guideline.CreateVersion(req.Name, req.Description, req.Note)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, v)
}

// handleListGuidelines GET /api/guidelines — 列出全部准则版本。
func (s *Server) handleListGuidelines(w http.ResponseWriter, r *http.Request) {
	list, err := s.app.Guideline.ListAll()
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, list)
}

// handleGetGuideline GET /api/guidelines/{id} — 读取版本详情。
func (s *Server) handleGetGuideline(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	v, err := s.app.Guideline.Get(id)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, v)
}

// handlePublishGuideline POST /api/guidelines/{id}/publish — 发布版本。
func (s *Server) handlePublishGuideline(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	v, err := s.app.Guideline.Publish(id)
	if err != nil {
		// 发布接口将状态机判断交给服务层。
		fail(w, err)
		return
	}
	ok(w, v)
}

type ambiguousReq struct {
	Note string `json:"note"`
}

// handleAmbiguousGuideline POST /api/guidelines/{id}/ambiguous — 标记歧义。
func (s *Server) handleAmbiguousGuideline(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	var req ambiguousReq
	_ = decode(r, &req)
	v, err := s.app.Guideline.MarkAmbiguous(id, req.Note)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, v)
}

// handleRevokeGuideline POST /api/guidelines/{id}/revoke — 废止版本。
func (s *Server) handleRevokeGuideline(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	var req ambiguousReq
	_ = decode(r, &req)
	v, err := s.app.Guideline.Revoke(id, req.Note)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, v)
}

// handleAddClause POST /api/guidelines/{id}/clauses — 添加条款。
func (s *Server) handleAddClause(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	var req clauseReq
	if err := decode(r, &req); err != nil {
		fail(w, model.ErrInvalidInput)
		return
	}
	c, err := s.app.Guideline.AddClause(id, req.ClauseNo, req.Layer, req.Title, req.Body, req.Scope)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, c)
}

// handleListClauses GET /api/guidelines/{id}/clauses — 条款列表。
func (s *Server) handleListClauses(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	clauses, err := s.app.Guideline.ListClauses(id)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, clauses)
}

type updateClauseReq struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Scope string `json:"scope"`
}

// handleUpdateClause PATCH /api/clauses/{id} — 修改条款。
func (s *Server) handleUpdateClause(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	var req updateClauseReq
	if err := decode(r, &req); err != nil {
		fail(w, model.ErrInvalidInput)
		return
	}
	c, err := s.app.Guideline.UpdateClause(id, req.Title, req.Body, req.Scope)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, c)
}

type impactReq struct {
	FromVersionID int64 `json:"from_version_id"`
}

// handleImpact POST /api/guidelines/{id}/impact — 准则影响分析。
func (s *Server) handleImpact(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	var req impactReq
	if err := decode(r, &req); err != nil {
		fail(w, model.ErrInvalidInput)
		return
	}
	if req.FromVersionID <= 0 {
		fail(w, model.ErrInvalidInput)
		return
	}
	report, err := s.app.Impact.Analyze(req.FromVersionID, id)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, report)
}
