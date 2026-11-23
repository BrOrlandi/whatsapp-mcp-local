package main

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Wails serves each request of the window on a goroutine of its own, with no
// recover: a panic in a panel handler used to close the desktop app. The
// request runs here the same way, on a bare goroutine, so a panic that
// escaped would end the test binary instead of failing one test.
func TestPanelPanicDoesNotCloseTheApp(t *testing.T) {
	var log bytes.Buffer
	s := &swapHandler{logger: slog.New(slog.NewTextHandler(&log, nil))}
	s.set(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.ServeHTTP(rec, httptest.NewRequest("GET", "/ajuda", nil))
	}()
	<-done

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `href="/"`) {
		t.Errorf("the page has no way back to the panel: %s", body)
	}
	for _, want := range []string{"/ajuda", "boom", "recover_test.go"} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("the log lacks %q: %s", want, log.String())
		}
	}
}

func TestPanelServesNormallyThroughTheRecover(t *testing.T) {
	s := &swapHandler{}
	s.set(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}
