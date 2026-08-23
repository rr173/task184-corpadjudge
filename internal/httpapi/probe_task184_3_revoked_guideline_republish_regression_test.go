package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"task184-corpadjudge/internal/service"
	"task184-corpadjudge/internal/store"
)

func TestRevokedGuidelineCannotBeRepublishedThroughAPI(t *testing.T) {
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app, err := service.New(db)
	if err != nil {
		t.Fatal(err)
	}
	h := New(app).Handler()
	create := httptest.NewRecorder()
	h.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/guidelines", bytes.NewBufferString(`{"name":"规范-3"}`)))
	if create.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	var envelope struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	id := envelope.Data.ID
	for _, path := range []string{"/api/guidelines/" + itoa(id) + "/publish", "/api/guidelines/" + itoa(id) + "/revoke"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{}`)))
		if w.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, w.Code, w.Body.String())
		}
	}
	republish := httptest.NewRecorder()
	h.ServeHTTP(republish, httptest.NewRequest(http.MethodPost, "/api/guidelines/"+itoa(id)+"/publish", nil))
	if republish.Code != http.StatusConflict {
		t.Fatalf("republish status=%d body=%s, want 409", republish.Code, republish.Body.String())
	}
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	buf := [20]byte{}
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
