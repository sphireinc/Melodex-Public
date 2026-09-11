package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStartupErrorHandlerServesMelodexRecoveryPage(t *testing.T) {
	handler := newStartupErrorHandler(assets)
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.Contains(contentType, "text/html") {
		t.Fatalf("expected html content type, got %q", contentType)
	}
	body := rec.Body.String()
	for _, want := range []string{"Melodex startup error", "The app UI could not load", "Reload app"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected body to contain %q", want)
		}
	}
}

func TestStartupErrorHandlerFallsBackToNotFoundForNonHTMLRequests(t *testing.T) {
	handler := newStartupErrorHandler(assets)
	req := httptest.NewRequest(http.MethodGet, "http://example.com/missing.css", nil)
	req.Header.Set("Accept", "text/css")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "404 page not found") {
		t.Fatalf("expected standard not found body, got %q", rec.Body.String())
	}
}
