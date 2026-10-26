package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConvertCatalog(t *testing.T) {
	input := []byte(`{"data":[{"id":"alpha","name":"Alpha","context_window":12345},{"id":"alpha"},{"id":null},{"id":42},{"id":"beta","display_name":"Beta","context_length":32768},null,{"id":" "}]}`)
	body, err := convertCatalog(input)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Models) != 2 {
		t.Fatalf("got %d models, want 2", len(result.Models))
	}
	if result.Models[0]["slug"] != "alpha" || result.Models[1]["slug"] != "beta" {
		t.Fatalf("unexpected IDs: %s", body)
	}
	if result.Models[1]["display_name"] != "Beta" {
		t.Errorf("display name incorrect: %s", body)
	}
	if result.Models[0]["context_window"] != float64(12345) || result.Models[1]["context_window"] != float64(32768) {
		t.Errorf("context windows: %s", body)
	}
	required := []string{"slug", "display_name", "base_instructions", "model_messages", "visibility", "truncation_policy", "supported_reasoning_levels", "input_modalities"}
	for _, field := range required {
		if _, ok := result.Models[0][field]; !ok {
			t.Errorf("missing %s", field)
		}
	}
	if result.Models[0]["visibility"] != "list" || result.Models[0]["supported_in_api"] != true {
		t.Errorf("missing picker metadata: %s", body)
	}
	msgs := result.Models[0]["model_messages"].(map[string]any)
	if msgs["instructions_template"] == "" {
		t.Fatal("empty Codex model instruction template")
	}
	if _, ok := result.Models[1]["context_window"]; !ok {
		t.Error("missing context window")
	}
}

func TestConvertCatalogInvalidInput(t *testing.T) {
	for _, input := range []string{`{}`, `{"data":null}`, `{"data":[]}`, `{"data":{}}`, `{`, `{"data":[{"id":42}]}`} {
		if _, err := convertCatalog([]byte(input)); err == nil {
			t.Errorf("accepted invalid input: %s", input)
		}
	}
}

func TestCatalogOutputBound(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"data":[`)
	for i := range 2200 {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":"test-model-%d"}`, i)
	}
	b.WriteString(`]}`)
	if b.Len() > maxBody {
		t.Fatal("test catalog input unexpectedly exceeded 1 MiB")
	}
	if _, err := convertCatalog([]byte(b.String())); err == nil {
		t.Fatal("expected oversized translated catalog error")
	}
}

func TestAuthorizationPriority(t *testing.T) {
	tests := []struct {
		name, explicit, fallback, request, expected string
	}{
		{name: "dedicated key wins", explicit: "private", fallback: "fallback", request: "Bearer incoming", expected: "Bearer private"},
		{name: "incoming overrides fallback", fallback: "fallback", request: "Bearer incoming", expected: "Bearer incoming"},
		{name: "fallback when no incoming", fallback: "fallback", expected: "Bearer fallback"},
		{name: "anonymous", expected: ""},
		{name: "non bearer header ignored", fallback: "fallback", request: "Basic something", expected: "Bearer fallback"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if actual := r.Header.Get("Authorization"); actual != tc.expected {
					t.Errorf("authorization = %q, want %q", actual, tc.expected)
				}
				_, _ = io.WriteString(w, `{"data":[{"id":"alpha"}]}`)
			}))
			defer upstream.Close()
			a, err := newAdapter(upstream.URL, tc.explicit, tc.fallback, 0)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodGet, "/models", nil)
			if tc.request != "" {
				req.Header.Set("Authorization", tc.request)
			}
			resp := httptest.NewRecorder()
			a.ServeHTTP(resp, req)
			if resp.Code != 200 {
				t.Fatalf("status %d: %s", resp.Code, resp.Body.String())
			}
		})
	}
}

func TestCacheWithFixedCredentialAndExpiry(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer fixed" {
			t.Error("incorrect fixed credential")
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"alpha"}]}`)
	}))
	defer upstream.Close()
	a, err := newAdapter(upstream.URL, "fixed", "", 40*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		request(t, a, "", 200)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("cache calls: got %d, want 1", got)
	}
	time.Sleep(55 * time.Millisecond)
	request(t, a, "", 200)
	if got := calls.Load(); got != 2 {
		t.Fatalf("expiry calls: got %d, want 2", got)
	}
}

func TestForwardedCredentialsBypassSharedCache(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		id := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if id == "" {
			id = "anonymous"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": id}}})
	}))
	defer upstream.Close()
	a, err := newAdapter(upstream.URL, "", "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ token, id string }{
		{"Bearer alice", "alice"}, {"Bearer bob", "bob"}, {"", "anonymous"},
		{"", "anonymous"}, {"Bearer alice", "alice"},
	} {
		response := request(t, a, tc.token, 200)
		if !strings.Contains(response, `"slug":"`+tc.id+`"`) {
			t.Errorf("incorrect catalog for %q: %s", tc.token, response)
		}
	}
	if got := calls.Load(); got != 4 {
		t.Errorf("got %d upstream requests, want 4", got)
	}
}

func TestConcurrentCacheMissIsCoalesced(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		time.Sleep(30 * time.Millisecond)
		_, _ = io.WriteString(w, `{"data":[{"id":"alpha"}]}`)
	}))
	defer upstream.Close()
	a, err := newAdapter(upstream.URL, "fixed", "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	const concurrency = 32
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, concurrency)
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			req := httptest.NewRequest(http.MethodGet, "/models", nil)
			resp := httptest.NewRecorder()
			a.ServeHTTP(resp, req)
			if resp.Code != 200 {
				errs <- fmt.Errorf("status %d: %s", resp.Code, resp.Body.String())
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("got %d upstream requests, want 1", got)
	}
}

func TestErrorMappingAndNoRedirect(t *testing.T) {
	for _, tc := range []struct{ upstream, expected int }{{401, 401}, {403, 403}, {429, 429}, {500, 502}, {302, 502}} {
		t.Run(fmt.Sprintf("%d", tc.upstream), func(t *testing.T) {
			var reached atomic.Bool
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/redirect" {
					reached.Store(true)
					w.WriteHeader(200)
					return
				}
				if tc.upstream == 302 {
					http.Redirect(w, r, "/redirect", 302)
					return
				}
				w.WriteHeader(tc.upstream)
			}))
			defer upstream.Close()
			a, err := newAdapter(upstream.URL, "", "", 0)
			if err != nil {
				t.Fatal(err)
			}
			request(t, a, "", tc.expected)
			if reached.Load() {
				t.Fatal("followed upstream redirect")
			}
		})
	}
}

func TestOversizeAndInvalidUpstream(t *testing.T) {
	for _, data := range []string{strings.Repeat("x", maxBody+1), `{"data":[]}`, `not json`} {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, data) }))
		a, err := newAdapter(upstream.URL, "", "", 0)
		if err != nil {
			t.Fatal(err)
		}
		request(t, a, "", 502)
		upstream.Close()
	}
}

func TestMissingEndpoint(t *testing.T) {
	a, err := newAdapter("http://127.0.0.1:1", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, method string }{{"/health", "GET"}, {"/models", "POST"}} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		resp := httptest.NewRecorder()
		a.ServeHTTP(resp, req)
		if resp.Code != 404 {
			t.Fatalf("unexpected status %d", resp.Code)
		}
	}
}

func request(t *testing.T, a *adapter, token string, expected int) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/models", nil)
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	resp := httptest.NewRecorder()
	a.ServeHTTP(resp, req)
	if resp.Code != expected {
		t.Fatalf("status %d, want %d: %s", resp.Code, expected, resp.Body.String())
	}
	if resp.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing cache control")
	}
	return resp.Body.String()
}

func BenchmarkCatalogConversion(b *testing.B) {
	var input strings.Builder
	input.WriteString(`{"data":[`)
	for i := range 40 {
		if i > 0 {
			input.WriteByte(',')
		}
		fmt.Fprintf(&input, `{"id":"cc/model-%d","context_window":128000}`, i)
	}
	input.WriteString(`]}`)
	raw := []byte(input.String())
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := convertCatalog(raw); err != nil {
			b.Fatal(err)
		}
	}
}
