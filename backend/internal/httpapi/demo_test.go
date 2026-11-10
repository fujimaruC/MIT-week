package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/amba/amba/backend/internal/models"
)

func TestDemoRouteServesValidDossier(t *testing.T) {
	rec := httptest.NewRecorder()
	testRouter(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/demo/demo-tiktok", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var d models.Dossier
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("body is not a Dossier: %v\n%s", err, rec.Body.String())
	}
	if d.SchemaVersion != 2 {
		t.Errorf("schema_version = %d", d.SchemaVersion)
	}
	if !d.IsDemo {
		t.Error("is_demo must be true for demo data")
	}
	if d.Verdict.Status == "" {
		t.Error("verdict.status is empty")
	}
}

func TestDemoRouteUnknownID(t *testing.T) {
	rec := httptest.NewRecorder()
	testRouter(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/demo/tidak-ada", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var env models.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("not the error envelope: %v", err)
	}
	if env.Error.Code == "" || env.Error.Message == "" {
		t.Errorf("incomplete envelope: %+v", env)
	}
}

func TestDemoRouteRejectsBadPattern(t *testing.T) {
	for _, id := range []string{"AB", "UPPER-CASE", "has space"} {
		rec := httptest.NewRecorder()
		testRouter(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/demo/"+url.PathEscape(id), nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("id %q: status = %d, want 404", id, rec.Code)
		}
	}
}
