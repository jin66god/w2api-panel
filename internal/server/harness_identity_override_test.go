package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/linguo2625469/workbuddy2api-panel/internal/auth"
	"github.com/linguo2625469/workbuddy2api-panel/internal/upstream"
)

func TestHarnessIdentityOverrideEndToEnd(t *testing.T) {
	cases := []struct {
		name    string
		mode    string
		enabled bool
		first   string
		length  int
	}{
		{"passthrough enabled", "passthrough", true, "You are CodeBuddy Code.\nKeep the rules.", 2},
		{"passthrough disabled", "passthrough", false, "You are an AI agent powered by DeepSeek Harness.\nKeep the rules.", 2},
		{"append enabled", "append", true, "You are CodeBuddy Code.\nKeep the rules.", 3},
		{"custom remains custom", "custom", true, "Gateway prompt", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sentBody []byte
			up := &upstream.Client{
				HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					sentBody, _ = io.ReadAll(r.Body)
					return &http.Response{
						StatusCode: 200,
						Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
						Body:       io.NopCloser(strings.NewReader(sseOK)),
					}, nil
				})},
				ChatBaseCN: "https://fake.example",
			}
			p := testPoolWith(&auth.Auth{UID: "u1", AccessToken: "at1", ExpiresAt: 9999999999})
			h := NewHandler(Config{
				Pool: p, Upstream: up,
				PromptMode: tc.mode, PromptText: "Gateway prompt",
				PromptIdentityOverrideEnabled: tc.enabled,
				PromptIdentityOverrideText:    "You are CodeBuddy Code.",
			})
			req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"glm-5.2","messages":[{"role":"system","content":"You are an AI agent powered by DeepSeek Harness.\nKeep the rules."},{"role":"user","content":"hi"}]}`))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != 200 {
				t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
			}
			var obj struct {
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
			}
			if err := json.Unmarshal(sentBody, &obj); err != nil {
				t.Fatalf("outbound JSON: %v (%s)", err, sentBody)
			}
			if len(obj.Messages) != tc.length || obj.Messages[0].Content != tc.first {
				t.Fatalf("unexpected outbound messages: %+v", obj.Messages)
			}
			if tc.mode == "append" && obj.Messages[1].Content != "Gateway prompt" {
				t.Fatalf("append should preserve inserted gateway prompt: %+v", obj.Messages)
			}
		})
	}
}
