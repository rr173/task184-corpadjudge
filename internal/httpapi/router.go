// Package httpapi 提供 RESTful HTTP 接口（路由前缀 /api）。
//
// 统一错误映射：业务层哨兵错误 → HTTP 状态码；
// 所有响应均为 JSON，成功返回 {"data": ...}，失败返回 {"error": ...}。
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/service"
)

// App 是 httpapi 依赖的服务门面。
type App = service.App

// Server 封装 HTTP 处理器与依赖。
type Server struct {
	app *App
	mux *http.ServeMux
}

// New 创建 HTTP 服务器并注册全部路由。
func New(app *App) *Server {
	s := &Server{app: app, mux: http.NewServeMux()}
	s.register()
	return s
}

// Handler 返回 http.Handler（供外部复用）。
func (s *Server) Handler() http.Handler { return s.mux }

// register 注册全部路由。
func (s *Server) register() {
	// 准则
	s.mux.HandleFunc("POST /api/guidelines", s.handleCreateGuideline)
	s.mux.HandleFunc("GET /api/guidelines", s.handleListGuidelines)
	s.mux.HandleFunc("GET /api/guidelines/{id}", s.handleGetGuideline)
	s.mux.HandleFunc("POST /api/guidelines/{id}/publish", s.handlePublishGuideline)
	s.mux.HandleFunc("POST /api/guidelines/{id}/ambiguous", s.handleAmbiguousGuideline)
	s.mux.HandleFunc("POST /api/guidelines/{id}/revoke", s.handleRevokeGuideline)
	s.mux.HandleFunc("POST /api/guidelines/{id}/clauses", s.handleAddClause)
	s.mux.HandleFunc("GET /api/guidelines/{id}/clauses", s.handleListClauses)
	s.mux.HandleFunc("PATCH /api/clauses/{id}", s.handleUpdateClause)
	s.mux.HandleFunc("POST /api/guidelines/{id}/impact", s.handleImpact)

	// 语料片段
	s.mux.HandleFunc("POST /api/spans", s.handleCreateSpan)
	s.mux.HandleFunc("GET /api/spans", s.handleListSpans)
	s.mux.HandleFunc("GET /api/spans/{id}", s.handleGetSpan)

	// 标注
	s.mux.HandleFunc("POST /api/spans/{id}/annotations", s.handleDraftAnnotation)
	s.mux.HandleFunc("PATCH /api/annotations/{id}", s.handleUpdateAnnotation)
	s.mux.HandleFunc("POST /api/annotations/{id}/submit", s.handleSubmitAnnotation)
	s.mux.HandleFunc("GET /api/annotations", s.handleListAnnotations)

	// 分歧
	s.mux.HandleFunc("GET /api/disputes", s.handleListDisputes)
	s.mux.HandleFunc("GET /api/disputes/{id}", s.handleGetDispute)
	s.mux.HandleFunc("POST /api/disputes/{id}/matrix", s.handleMatrix)
	s.mux.HandleFunc("POST /api/disputes/{id}/refs", s.handleReferenceClauses)

	// 裁决
	s.mux.HandleFunc("POST /api/cases", s.handleCreateCase)
	s.mux.HandleFunc("GET /api/cases", s.handleListCases)
	s.mux.HandleFunc("GET /api/cases/{id}", s.handleGetCase)
	s.mux.HandleFunc("POST /api/cases/{id}/decide", s.handleDecideCase)
	s.mux.HandleFunc("POST /api/cases/{id}/formalize", s.handleFormalizeCase)

	// 基准
	s.mux.HandleFunc("POST /api/baselines", s.handleCreateSnapshot)
	s.mux.HandleFunc("GET /api/baselines", s.handleListSnapshots)
	s.mux.HandleFunc("GET /api/baselines/{id}", s.handleGetSnapshot)
	s.mux.HandleFunc("POST /api/baselines/{id}/freeze", s.handleFreezeSnapshot)
	s.mux.HandleFunc("GET /api/baselines/{id}/export", s.handleExportSnapshot)

	// 重审
	s.mux.HandleFunc("GET /api/rereviews", s.handleListRereviews)
	s.mux.HandleFunc("POST /api/rereviews/{id}/resolve", s.handleResolveRereview)

	// 统计与自检
	s.mux.HandleFunc("GET /api/stats", s.handleStats)
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("POST /api/selfcheck", s.handleSelfCheck)
}

// writeJSON 输出 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("write json: %v", err)
	}
}

// ok 输出成功响应。
func ok(w http.ResponseWriter, data any) { writeJSON(w, http.StatusOK, map[string]any{"data": data}) }

// fail 将业务错误映射为 HTTP 错误响应。
func fail(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, model.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, model.ErrInvalidInput):
		status = http.StatusBadRequest
	case errors.Is(err, model.ErrDuplicate):
		status = http.StatusConflict
	case errors.Is(err, model.ErrInvalidState):
		status = http.StatusConflict
	case errors.Is(err, model.ErrFrozen):
		status = http.StatusConflict
	case errors.Is(err, model.ErrVersionConflict):
		status = http.StatusConflict
	case errors.Is(err, model.ErrStaleGuideline):
		status = http.StatusConflict
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// pathID 从请求路径变量解析 int64。
func pathID(r *http.Request, key string) (int64, error) {
	v := r.PathValue(key)
	if v == "" {
		return 0, model.ErrInvalidInput
	}
	var id int64
	if _, err := fmt.Sscan(v, &id); err != nil {
		return 0, model.ErrInvalidInput
	}
	return id, nil
}

// decode 解码 JSON 请求体到目标结构。
func decode(r *http.Request, dst any) error {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		return err
	}
	return nil
}

// queryInt 解析查询参数中的整数，缺省返回 def。
func queryInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscan(v, &n); err != nil {
		return def
	}
	return n
}
