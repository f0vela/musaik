package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"musaik/internal/library"
	"musaik/internal/model"
	"musaik/internal/player"
)

func newRemoteTestApp() (*app, *remoteServer) {
	a := &app{
		store:       library.NewStore(),
		player:      player.New(),
		volumeLevel: 80,
		playIndex:   -1,
	}
	for _, s := range []model.Song{
		{ID: "1", Title: "Alpha", Artist: "Foo", Album: "One"},
		{ID: "2", Title: "Beta", Artist: "Foo", Album: "Two"},
		{ID: "3", Title: "Gamma", Artist: "Bar", Album: "Three"},
	} {
		a.store.Songs = append(a.store.Songs, s)
	}
	return a, &remoteServer{port: 4000, token: "secret"}
}

func remoteDo(h http.Handler, method, target, host, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.Host = host
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRemoteAuthAndHost(t *testing.T) {
	a, rs := newRemoteTestApp()
	h := a.remoteHandler(rs)

	if rec := remoteDo(h, "GET", "/state", "127.0.0.1:4000", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin token: %d", rec.Code)
	}
	if rec := remoteDo(h, "GET", "/state", "127.0.0.1:4000", "wrong"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("token malo: %d", rec.Code)
	}
	if rec := remoteDo(h, "GET", "/state", "evil.example:4000", "secret"); rec.Code != http.StatusForbidden {
		t.Fatalf("host ajeno: %d", rec.Code)
	}
	if rec := remoteDo(h, "GET", "/state", "localhost:4000", "secret"); rec.Code != http.StatusOK {
		t.Fatalf("válido: %d", rec.Code)
	}
}

func TestRemoteSearchLimit(t *testing.T) {
	a, rs := newRemoteTestApp()
	h := a.remoteHandler(rs)

	var got []remoteSong
	rec := remoteDo(h, "GET", "/search?q=foo&limit=1", "127.0.0.1:4000", "secret")
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got) != 1 || got[0].ID != "1" {
		t.Fatalf("limit: %v %s", err, rec.Body.String())
	}
	rec = remoteDo(h, "GET", "/search?q=bar", "127.0.0.1:4000", "secret")
	got = nil
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got) != 1 || got[0].Title != "Gamma" {
		t.Fatalf("query: %v %s", err, rec.Body.String())
	}
}

func TestRemoteQueueAndValidation(t *testing.T) {
	a, rs := newRemoteTestApp()
	h := a.remoteHandler(rs)

	if rec := remoteDo(h, "POST", "/queue/2", "127.0.0.1:4000", "secret"); rec.Code != http.StatusOK {
		t.Fatalf("queue: %d", rec.Code)
	}
	if len(a.queueSongIDs) != 1 || a.queueSongIDs[0] != "2" {
		t.Fatalf("cola: %v", a.queueSongIDs)
	}
	if rec := remoteDo(h, "POST", "/queue/nope", "127.0.0.1:4000", "secret"); rec.Code != http.StatusBadRequest {
		t.Fatalf("id inexistente: %d", rec.Code)
	}
	if rec := remoteDo(h, "POST", "/volume?level=500", "127.0.0.1:4000", "secret"); rec.Code != http.StatusBadRequest {
		t.Fatalf("volumen inválido: %d", rec.Code)
	}
	if rec := remoteDo(h, "GET", "/play", "127.0.0.1:4000", "secret"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET en acción: %d", rec.Code)
	}
}
