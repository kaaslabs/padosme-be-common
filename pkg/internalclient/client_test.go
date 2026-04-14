package internalclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDoJSONSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Service-Token") != "tok" {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("X-Caller-Service") != "me" {
			http.Error(w, "bad caller", http.StatusBadRequest)
			return
		}
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"echo": in["hello"]})
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, ServiceToken: "tok", CallerName: "me"})
	var out map[string]string
	if err := c.DoJSON(context.Background(), "POST", "/", map[string]string{"hello": "world"}, &out); err != nil {
		t.Fatalf("DoJSON: %v", err)
	}
	if out["echo"] != "world" {
		t.Fatalf("unexpected response: %+v", out)
	}
}

func TestDoJSONErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, ServiceToken: "tok"})
	err := c.DoJSON(context.Background(), "GET", "/x", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var ie *Error
	if !errorsAs(err, &ie) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if ie.Status != http.StatusForbidden {
		t.Fatalf("status = %d", ie.Status)
	}
}

// tiny errors.As wrapper to avoid pulling errors import at top.
func errorsAs(err error, target any) bool {
	// Using standard library indirectly; keep test self-contained.
	type asI interface{ As(any) bool }
	if e, ok := err.(*Error); ok {
		if t, ok := target.(**Error); ok {
			*t = e
			return true
		}
	}
	_ = asI(nil)
	return false
}
