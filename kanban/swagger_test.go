package kanban

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSwaggerRoutes(t *testing.T) {
	router := NewRouter(nil)

	specRecorder := httptest.NewRecorder()
	router.ServeHTTP(specRecorder, httptest.NewRequest(http.MethodGet, "/swagger/openapi.json", nil))

	if specRecorder.Code != http.StatusOK {
		t.Fatalf("expected OpenAPI status %d, got %d", http.StatusOK, specRecorder.Code)
	}
	if !strings.Contains(specRecorder.Body.String(), `"name": "Authorization"`) {
		t.Fatalf("expected Authorization API key scheme in OpenAPI document")
	}

	uiRecorder := httptest.NewRecorder()
	router.ServeHTTP(uiRecorder, httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil))

	if uiRecorder.Code != http.StatusOK {
		t.Fatalf("expected Swagger UI status %d, got %d", http.StatusOK, uiRecorder.Code)
	}
	if !strings.Contains(uiRecorder.Body.String(), "SwaggerUIBundle") {
		t.Fatalf("expected Swagger UI bundle in HTML")
	}
}
