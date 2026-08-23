package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"task184-corpadjudge/internal/model"
	"task184-corpadjudge/internal/service"
	"task184-corpadjudge/internal/store"
)

func TestFrozenSpanFingerprintRemainsIdempotentThroughAPI(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app, err := service.New(db)
	if err != nil {
		t.Fatal(err)
	}
	srv := New(app).Handler()
	payload := []byte(`{"doc_id":"doc-frozen","start_offset":0,"end_offset":4,"text":"测试文本","layer":"pos"}`)

	first := httptest.NewRecorder()
	srv.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/api/spans", bytes.NewReader(payload)))
	if first.Code != http.StatusOK {
		t.Fatalf("first create status=%d body=%s", first.Code, first.Body.String())
	}
	var firstEnvelope struct {
		Data model.CorpusSpan `json:"data"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstEnvelope); err != nil {
		t.Fatal(err)
	}
	if err := app.Corpus.MarkStatus(firstEnvelope.Data.ID, model.SpanConsistent); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Corpus.Freeze(firstEnvelope.Data.ID); err != nil {
		t.Fatal(err)
	}

	second := httptest.NewRecorder()
	srv.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/api/spans", bytes.NewReader(payload)))
	if second.Code != http.StatusOK {
		t.Fatalf("frozen duplicate status=%d body=%s, want 200", second.Code, second.Body.String())
	}
	var secondEnvelope struct {
		Data model.CorpusSpan `json:"data"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondEnvelope); err != nil {
		t.Fatal(err)
	}
	if secondEnvelope.Data.ID != firstEnvelope.Data.ID || secondEnvelope.Data.Status != model.SpanFrozen {
		t.Fatalf("frozen duplicate returned %+v, want original id=%d and frozen status", secondEnvelope.Data, firstEnvelope.Data.ID)
	}
}
