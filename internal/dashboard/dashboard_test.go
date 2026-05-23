package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/xzxiong/ai-coding/internal/storage"
)

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestHandler_APIUsage_Empty(t *testing.T) {
	h := NewHandler(newTestStore(t))
	req := httptest.NewRequest(http.MethodGet, "/dashboard/api/usage", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if int(resp["total_requests"].(float64)) != 0 {
		t.Errorf("expected 0 requests, got %v", resp["total_requests"])
	}
}

func TestHandler_APIUsage_Aggregation(t *testing.T) {
	store := newTestStore(t)
	store.Record(storage.UsageRecord{
		Timestamp: time.Now(), Model: "gpt-4o",
		InputTokens: 10, OutputTokens: 20, TotalTokens: 30, Duration: 100,
	})
	store.Record(storage.UsageRecord{
		Timestamp: time.Now(), Model: "deepseek-v4-pro",
		InputTokens: 5, OutputTokens: 15, TotalTokens: 20, Duration: 200,
	})

	h := NewHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/dashboard/api/usage", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if int(resp["total_requests"].(float64)) != 2 {
		t.Errorf("expected 2 requests, got %v", resp["total_requests"])
	}
	if int(resp["total_input_tokens"].(float64)) != 15 {
		t.Errorf("expected 15 input tokens, got %v", resp["total_input_tokens"])
	}
	if int(resp["total_tokens"].(float64)) != 50 {
		t.Errorf("expected 50 total tokens, got %v", resp["total_tokens"])
	}
	if int(resp["avg_duration_ms"].(float64)) != 150 {
		t.Errorf("expected avg 150ms, got %v", resp["avg_duration_ms"])
	}

	breakdown := resp["model_breakdown"].(map[string]any)
	if int(breakdown["gpt-4o"].(float64)) != 1 {
		t.Errorf("expected gpt-4o count 1")
	}
	if int(breakdown["deepseek-v4-pro"].(float64)) != 1 {
		t.Errorf("expected deepseek-v4-pro count 1")
	}
}

func TestHandler_APIUsage_RangeFilter(t *testing.T) {
	store := newTestStore(t)
	store.Record(storage.UsageRecord{
		Timestamp: time.Now().Add(-2 * time.Hour), Model: "old",
		InputTokens: 100, OutputTokens: 200, TotalTokens: 300,
	})
	store.Record(storage.UsageRecord{
		Timestamp: time.Now(), Model: "recent",
		InputTokens: 10, OutputTokens: 20, TotalTokens: 30,
	})

	h := NewHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/dashboard/api/usage?range=1h", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if int(resp["total_requests"].(float64)) != 1 {
		t.Errorf("expected 1 request in 1h range, got %v", resp["total_requests"])
	}
}

func TestHandler_APIUsage_Pagination(t *testing.T) {
	store := newTestStore(t)
	for i := 0; i < 5; i++ {
		store.Record(storage.UsageRecord{
			Timestamp:   time.Now(),
			Model:       "gpt-4o",
			InputTokens: 10 * (i + 1),
			OutputTokens: 5,
			TotalTokens: 10*(i+1) + 5,
			Duration:    100,
		})
	}

	h := NewHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/api/usage?page=1&page_size=2", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if int(resp["total_requests"].(float64)) != 5 {
		t.Errorf("expected 5 total requests, got %v", resp["total_requests"])
	}
	if int(resp["total_records"].(float64)) != 5 {
		t.Errorf("expected 5 total records, got %v", resp["total_records"])
	}
	if int(resp["total_pages"].(float64)) != 3 {
		t.Errorf("expected 3 total pages, got %v", resp["total_pages"])
	}
	if int(resp["page"].(float64)) != 1 {
		t.Errorf("expected page 1, got %v", resp["page"])
	}
	records := resp["records"].([]any)
	if len(records) != 2 {
		t.Errorf("expected 2 records on page 1, got %d", len(records))
	}

	req = httptest.NewRequest(http.MethodGet, "/dashboard/api/usage?page=3&page_size=2", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	json.Unmarshal(rec.Body.Bytes(), &resp)
	records = resp["records"].([]any)
	if len(records) != 1 {
		t.Errorf("expected 1 record on page 3, got %d", len(records))
	}
}

func TestHandler_APIUsage_PaginationBounds(t *testing.T) {
	store := newTestStore(t)
	store.Record(storage.UsageRecord{
		Timestamp: time.Now(), Model: "gpt-4o",
		InputTokens: 10, OutputTokens: 5, TotalTokens: 15,
	})

	h := NewHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/api/usage?page=999&page_size=10", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)

	records := resp["records"].([]any)
	if len(records) != 0 {
		t.Errorf("expected 0 records for out-of-range page, got %d", len(records))
	}
}

func TestHandler_DashboardPage(t *testing.T) {
	h := NewHandler(newTestStore(t))
	req := httptest.NewRequest(http.MethodGet, "/dashboard/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Errorf("expected html content type, got %s", ct)
	}
}
