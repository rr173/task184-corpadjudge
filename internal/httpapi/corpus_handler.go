package httpapi

import (
	"net/http"

	"task184-corpadjudge/internal/model"
)

type createSpanReq struct {
	DocID string      `json:"doc_id"`
	Start int         `json:"start_offset"`
	End   int         `json:"end_offset"`
	Text  string      `json:"text"`
	Layer model.Layer `json:"layer"`
}

// handleCreateSpan POST /api/spans — 创建语料片段（指纹幂等）。
func (s *Server) handleCreateSpan(w http.ResponseWriter, r *http.Request) {
	var req createSpanReq
	if err := decode(r, &req); err != nil {
		fail(w, model.ErrInvalidInput)
		return
	}
	sp, err := s.app.Corpus.Create(req.DocID, req.Start, req.End, req.Text, req.Layer)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, sp)
}

// handleListSpans GET /api/spans?status=&limit=&offset= — 片段列表。
func (s *Server) handleListSpans(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)
	list, err := s.app.Corpus.List(status, limit, offset)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, list)
}

// handleGetSpan GET /api/spans/{id} — 片段详情。
func (s *Server) handleGetSpan(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	sp, err := s.app.Corpus.Get(id)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, sp)
}

type draftAnnotationReq struct {
	Annotator      string      `json:"annotator"`
	Layer          model.Layer `json:"layer"`
	Label          string      `json:"label"`
	StartOffset    int         `json:"start_offset"`
	EndOffset      int         `json:"end_offset"`
	GuidelineVerID int64       `json:"guideline_version_id"`
}

// handleDraftAnnotation POST /api/spans/{id}/annotations — 创建并提交标注（工作流：草稿→提交→归并）。
func (s *Server) handleDraftAnnotation(w http.ResponseWriter, r *http.Request) {
	spanID, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	var req draftAnnotationReq
	if err := decode(r, &req); err != nil {
		fail(w, model.ErrInvalidInput)
		return
	}
	ann, merge, err := s.app.Workflow(spanID, req.Annotator, req.Layer, req.Label,
		req.StartOffset, req.EndOffset, req.GuidelineVerID)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, map[string]any{"annotation": ann, "merge": merge})
}

type updateAnnotationReq struct {
	Label       string `json:"label"`
	StartOffset int    `json:"start_offset"`
	EndOffset   int    `json:"end_offset"`
}

// handleUpdateAnnotation PATCH /api/annotations/{id} — 更新草稿标注。
func (s *Server) handleUpdateAnnotation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	var req updateAnnotationReq
	if err := decode(r, &req); err != nil {
		fail(w, model.ErrInvalidInput)
		return
	}
	ann, err := s.app.Annotation.UpdateDraft(id, req.Label, req.StartOffset, req.EndOffset)
	if err != nil {
		// 更新失败时统一使用领域错误映射。
		fail(w, err)
		return
	}
	ok(w, ann)
}

// handleSubmitAnnotation POST /api/annotations/{id}/submit — 提交标注。
func (s *Server) handleSubmitAnnotation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	ann, err := s.app.Annotation.Submit(id)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, ann)
}

// handleListAnnotations GET /api/annotations?span_id=&layer= — 标注列表。
func (s *Server) handleListAnnotations(w http.ResponseWriter, r *http.Request) {
	spanID := queryInt(r, "span_id", 0)
	layer := r.URL.Query().Get("layer")
	if spanID <= 0 {
		fail(w, model.ErrInvalidInput)
		return
	}
	list, err := s.app.Annotation.ListBySpan(int64(spanID), layer)
	if err != nil {
		fail(w, err)
		return
	}
	ok(w, list)
}
