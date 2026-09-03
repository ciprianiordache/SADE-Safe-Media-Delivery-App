package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusCreated, map[string]int{"n": 1})
	if w.Code != http.StatusCreated {
		t.Errorf("status = %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("content-type = %q", ct)
	}
	if strings.TrimSpace(w.Body.String()) != `{"n":1}` {
		t.Errorf("body = %q", w.Body.String())
	}
}

func TestError(t *testing.T) {
	w := httptest.NewRecorder()
	Error(w, http.StatusNotFound, "nope")
	var got ErrorBody
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusNotFound || got.Error != "nope" {
		t.Errorf("got %d %+v", w.Code, got)
	}
}

func TestDecodeJSON(t *testing.T) {
	type in struct {
		Name string `json:"name"`
	}

	t.Run("ok", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"a"}`))
		var v in
		if err := DecodeJSON(httptest.NewRecorder(), r, &v, 1<<10); err != nil {
			t.Fatal(err)
		}
		if v.Name != "a" {
			t.Errorf("v = %+v", v)
		}
	})

	t.Run("unknown field", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"a","x":1}`))
		if err := DecodeJSON(httptest.NewRecorder(), r, &in{}, 1<<10); err == nil {
			t.Error("expected error for unknown field")
		}
	})

	t.Run("trailing data", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"a"}{}`))
		if err := DecodeJSON(httptest.NewRecorder(), r, &in{}, 1<<10); err == nil {
			t.Error("expected error for trailing data")
		}
	})

	t.Run("too large", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"`+strings.Repeat("x", 5000)+`"}`))
		if err := DecodeJSON(httptest.NewRecorder(), r, &in{}, 100); err == nil {
			t.Error("expected error for oversize body")
		}
	})
}

func TestPage(t *testing.T) {
	for _, tc := range []struct {
		query                 string
		wantOffset, wantLimit int
	}{
		{"", 0, 50},
		{"limit=10&offset=20", 20, 10},
		{"limit=0", 0, 1},
		{"limit=9999", 0, 200},
		{"offset=-5", 0, 50},
		{"limit=abc", 0, 50},
	} {
		r := httptest.NewRequest("GET", "/?"+tc.query, nil)
		o, l := Page(r, 50, 200)
		if o != tc.wantOffset || l != tc.wantLimit {
			t.Errorf("Page(?%s) = (%d,%d), want (%d,%d)", tc.query, o, l, tc.wantOffset, tc.wantLimit)
		}
	}
}
