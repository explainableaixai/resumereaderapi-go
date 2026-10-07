package resumereaderapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeBatchesAndErrors(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["api_key"] != "good" {
			_, _ = w.Write([]byte(`{"status":401}`))
			return
		}
		n := len(in["skills"].([]any))
		results := make([]map[string]any, n)
		for i := range results {
			results[i] = map[string]any{"input": "x"}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": 200, "results": results, "credits_used": float64(n) / 10, "remaining_credits": 5.0})
	}))
	defer srv.Close()

	c := New("good")
	c.Base = srv.URL + "/"
	skills := make([]string, 150)
	out, err := c.NormalizeSkills(context.Background(), skills)
	if err != nil || calls != 2 || len(out.Results) != 150 || out.CreditsUsed < 14.99 || out.CreditsUsed > 15.01 {
		t.Fatalf("unexpected: %v calls=%d %+v", err, calls, out)
	}
	bad := New("bad")
	bad.Base = srv.URL + "/"
	if _, err := bad.NormalizeSkills(context.Background(), []string{"js"}); err == nil {
		t.Fatal("expected error")
	}
}
