// OmniRoute model discovery adapter for Codex CLI.
// All inference traffic continues to go directly to OmniRoute.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxBody          = 1 << 20
	defaultURL       = "http://127.0.0.1:20128/v1/models?prefix=alias"
	baseInstructions = "You are a coding assistant running in Codex CLI. Follow the user's instructions, use the available tools when appropriate, and report changes and tests accurately."
)

type modelInfo struct {
	Slug             string `json:"slug"`
	DisplayName      string `json:"display_name"`
	Description      any    `json:"description"`
	BaseInstructions string `json:"base_instructions"`
	ModelMessages    struct {
		InstructionsTemplate string `json:"instructions_template"`
	} `json:"model_messages"`
	DefaultReasoningLevel    any      `json:"default_reasoning_level"`
	SupportedReasoningLevels []string `json:"supported_reasoning_levels"`
	ShellType                string   `json:"shell_type"`
	Visibility               string   `json:"visibility"`
	SupportedInAPI           bool     `json:"supported_in_api"`
	Priority                 int      `json:"priority"`
	AvailabilityNux          any      `json:"availability_nux"`
	Upgrade                  any      `json:"upgrade"`
	SupportVerbosity         bool     `json:"support_verbosity"`
	DefaultVerbosity         any      `json:"default_verbosity"`
	ApplyPatchToolType       any      `json:"apply_patch_tool_type"`
	TruncationPolicy         struct {
		Mode  string `json:"mode"`
		Limit int    `json:"limit"`
	} `json:"truncation_policy"`
	ExperimentalSupportedTools []string `json:"experimental_supported_tools"`
	InputModalities            []string `json:"input_modalities"`
	ContextWindow              *int64   `json:"context_window,omitempty"`
}

type upstreamEntry struct {
	ID            json.RawMessage `json:"id"`
	Name          json.RawMessage `json:"name"`
	DisplayName   json.RawMessage `json:"display_name"`
	ContextWindow json.RawMessage `json:"context_window"`
	ContextLength json.RawMessage `json:"context_length"`
}

func optionalString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil && strings.TrimSpace(s) != "" {
		return s
	}
	return ""
}

func positiveContext(raw json.RawMessage) int64 {
	if len(raw) == 0 || !((raw[0] >= '0' && raw[0] <= '9') || raw[0] == '-') {
		return 0
	}
	n, err := strconv.ParseFloat(string(raw), 64)
	if err != nil || n <= 0 || n > 9007199254740991 || math.Trunc(n) != n {
		return 0
	}
	return int64(n)
}

func convertCatalog(raw []byte) ([]byte, error) {
	var upstream struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &upstream); err != nil || upstream.Data == nil {
		return nil, errors.New("expected OpenAI models object with a data array")
	}
	capacity := min(len(upstream.Data), 64)
	models := make([]modelInfo, 0, capacity)
	seen := make(map[string]struct{}, capacity)
	for _, item := range upstream.Data {
		var entry upstreamEntry
		if json.Unmarshal(item, &entry) != nil {
			continue
		}
		id := optionalString(entry.ID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		name := optionalString(entry.Name)
		if name == "" {
			name = optionalString(entry.DisplayName)
		}
		if name == "" {
			name = id
		}
		m := modelInfo{
			Slug: id, DisplayName: name, BaseInstructions: baseInstructions,
			SupportedReasoningLevels: []string{}, ShellType: "shell_command",
			Visibility: "list", SupportedInAPI: true, Priority: len(models) + 1,
			ExperimentalSupportedTools: []string{}, InputModalities: []string{"text"},
		}
		m.ModelMessages.InstructionsTemplate = baseInstructions
		m.TruncationPolicy.Mode, m.TruncationPolicy.Limit = "tokens", 10000
		contextSize := positiveContext(entry.ContextWindow)
		if contextSize == 0 {
			contextSize = positiveContext(entry.ContextLength)
		}
		if contextSize > 0 {
			m.ContextWindow = &contextSize
		}
		models = append(models, m)
	}
	if len(models) == 0 {
		return nil, errors.New("OmniRoute returned no usable models")
	}
	out, err := json.Marshal(struct {
		Models []modelInfo `json:"models"`
	}{models})
	if err != nil {
		return nil, err
	}
	if len(out) > maxBody {
		return nil, errors.New("model catalog exceeds Codex 1 MiB limit")
	}
	return out, nil
}

type result struct {
	body   []byte
	status int
}
type flight struct {
	done   chan struct{}
	result result
}
type catalogCache struct {
	mu       sync.Mutex
	payload  []byte
	expires  time.Time
	inFlight *flight
	ttl      time.Duration
}

func (c *catalogCache) get(ctx context.Context, fetch func() result) result {
	c.mu.Lock()
	if c.payload != nil && time.Now().Before(c.expires) {
		res := result{body: c.payload, status: http.StatusOK}
		c.mu.Unlock()
		return res
	}
	if current := c.inFlight; current != nil {
		c.mu.Unlock()
		select {
		case <-current.done:
			return current.result
		case <-ctx.Done():
			return result{status: http.StatusBadGateway, body: errorJSON("upstream request canceled")}
		}
	}
	current := &flight{done: make(chan struct{})}
	c.inFlight = current
	c.mu.Unlock()

	// Share a single upstream fetch among callers. Its own HTTP timeout
	// prevents the first client disconnecting from aborting other callers.
	fetched := fetch()
	c.mu.Lock()
	if fetched.status == http.StatusOK {
		c.payload = fetched.body
		c.expires = time.Now().Add(c.ttl)
	}
	current.result = fetched
	c.inFlight = nil
	close(current.done)
	c.mu.Unlock()
	return fetched
}

type adapter struct {
	upstream    string
	client      *http.Client
	explicitKey string
	fallbackKey string
	cache       *catalogCache
}

func newAdapter(upstream, explicitKey, fallbackKey string, ttl time.Duration) (*adapter, error) {
	u, err := url.Parse(upstream)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("OMNIROUTE_MODELS_URL must be an absolute http(s) URL")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 8
	transport.MaxIdleConnsPerHost = 4
	transport.MaxConnsPerHost = 16
	a := &adapter{
		upstream:    upstream,
		explicitKey: explicitKey,
		fallbackKey: fallbackKey,
		client: &http.Client{
			Transport:     transport,
			Timeout:       10 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	if ttl > 0 {
		a.cache = &catalogCache{ttl: ttl}
	}
	return a, nil
}

func errorJSON(message string) []byte {
	body, _ := json.Marshal(struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}{
		Error: struct {
			Message string `json:"message"`
		}{Message: message},
	})
	return body
}

func (a *adapter) fetch(ctx context.Context, authorization string) result {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.upstream, nil)
	if err != nil {
		return result{status: 502, body: errorJSON("invalid or unavailable OmniRoute catalog")}
	}
	req.Header.Set("Accept", "application/json")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		log.Printf("catalog fetch: %v", err)
		return result{status: 502, body: errorJSON("invalid or unavailable OmniRoute catalog")}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		status := http.StatusBadGateway
		if resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 429 {
			status = resp.StatusCode
		}
		return result{status: status, body: errorJSON(fmt.Sprintf("OmniRoute returned HTTP %d", resp.StatusCode))}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		log.Printf("catalog read failed or exceeded 1 MiB")
		return result{status: 502, body: errorJSON("invalid or unavailable OmniRoute catalog")}
	}
	translated, err := convertCatalog(body)
	if err != nil {
		log.Printf("catalog conversion: %v", err)
		return result{status: 502, body: errorJSON("invalid or unavailable OmniRoute catalog")}
	}
	return result{status: http.StatusOK, body: translated}
}

func send(w http.ResponseWriter, res result) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(res.body)))
	w.WriteHeader(res.status)
	_, _ = w.Write(res.body)
}

func (a *adapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.Path != "/models" {
		send(w, result{status: 404, body: []byte(`{"error":{"message":"not found"}}`)})
		return
	}
	authorization := ""
	perCaller := false
	if a.explicitKey != "" {
		authorization = "Bearer " + a.explicitKey
	} else if incoming := r.Header.Get("Authorization"); len(incoming) >= 7 && strings.EqualFold(incoming[:7], "Bearer ") && strings.TrimSpace(incoming[7:]) != "" {
		authorization = incoming
		perCaller = true
	} else if a.fallbackKey != "" {
		authorization = "Bearer " + a.fallbackKey
	}
	if perCaller || a.cache == nil {
		send(w, a.fetch(r.Context(), authorization))
		return
	}
	send(w, a.cache.get(r.Context(), func() result { return a.fetch(context.Background(), authorization) }))
}

func main() {
	upstream := os.Getenv("OMNIROUTE_MODELS_URL")
	if upstream == "" {
		upstream = defaultURL
	}
	ttlSeconds := 30
	if raw := os.Getenv("CACHE_TTL_SECONDS"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 3600 {
			log.Fatal("CACHE_TTL_SECONDS must be between 0 and 3600")
		}
		ttlSeconds = n
	}
	a, err := newAdapter(upstream, os.Getenv("OMNIROUTE_MODELS_API_KEY"), os.Getenv("OMNIROUTE_API_KEY"), time.Duration(ttlSeconds)*time.Second)
	if err != nil {
		log.Fatal(err)
	}
	host := os.Getenv("HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8787"
	}
	s := &http.Server{
		Addr: host + ":" + port, Handler: a,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    8192,
	}
	log.Printf("Codex models adapter: http://%s:%s/models -> %s (cache %ds)", host, port, upstream, ttlSeconds)
	log.Fatal(s.ListenAndServe())
}
