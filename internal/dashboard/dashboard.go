package dashboard

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/xzxiong/ai-coding/internal/storage"
)

type Handler struct {
	store *storage.Store
}

func NewHandler(store *storage.Store) *Handler {
	return &Handler{store: store}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/dashboard/api/usage" {
		h.apiUsage(w, r)
		return
	}
	h.page(w, r)
}

func (h *Handler) apiUsage(w http.ResponseWriter, r *http.Request) {
	rangeParam := r.URL.Query().Get("range")
	var records []storage.UsageRecord

	now := time.Now()
	switch rangeParam {
	case "1h":
		records = h.store.Since(now.Add(-1 * time.Hour))
	case "24h":
		records = h.store.Since(now.Add(-24 * time.Hour))
	case "7d":
		records = h.store.Since(now.Add(-7 * 24 * time.Hour))
	case "30d":
		records = h.store.Since(now.Add(-30 * 24 * time.Hour))
	default:
		records = h.store.Records()
	}

	type summary struct {
		TotalRequests  int                   `json:"total_requests"`
		TotalInput     int                   `json:"total_input_tokens"`
		TotalOutput    int                   `json:"total_output_tokens"`
		TotalTokens    int                   `json:"total_tokens"`
		AvgDuration    int64                 `json:"avg_duration_ms"`
		ModelBreakdown map[string]int        `json:"model_breakdown"`
		Records        []storage.UsageRecord `json:"records"`
	}

	s := summary{
		ModelBreakdown: make(map[string]int),
		Records:        records,
	}

	var totalDuration int64
	for _, rec := range records {
		s.TotalRequests++
		s.TotalInput += rec.InputTokens
		s.TotalOutput += rec.OutputTokens
		s.TotalTokens += rec.TotalTokens
		totalDuration += rec.Duration
		s.ModelBreakdown[rec.Model]++
	}
	if s.TotalRequests > 0 {
		s.AvgDuration = totalDuration / int64(s.TotalRequests)
	}
	if s.Records == nil {
		s.Records = []storage.UsageRecord{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

func (h *Handler) page(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(dashboardHTML))
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Token Usage Dashboard</title>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f5f5f5; padding: 20px; }
.container { max-width: 1200px; margin: 0 auto; }
h1 { margin-bottom: 20px; color: #333; }
.toolbar { display: flex; gap: 8px; margin-bottom: 20px; align-items: center; }
.toolbar button { padding: 8px 16px; border: 1px solid #ddd; background: #fff; border-radius: 6px; cursor: pointer; font-size: 13px; }
.toolbar button.active { background: #2563eb; color: #fff; border-color: #2563eb; }
.toolbar button:hover { border-color: #2563eb; }
.cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 16px; margin-bottom: 24px; }
.card { background: #fff; border-radius: 8px; padding: 20px; box-shadow: 0 1px 3px rgba(0,0,0,0.1); }
.card .label { font-size: 12px; color: #666; text-transform: uppercase; letter-spacing: 0.5px; }
.card .value { font-size: 28px; font-weight: 700; color: #333; margin-top: 4px; }
.card .value.input { color: #2563eb; }
.card .value.output { color: #16a34a; }
.card .value.total { color: #9333ea; }
.section { background: #fff; border-radius: 8px; padding: 20px; box-shadow: 0 1px 3px rgba(0,0,0,0.1); margin-bottom: 24px; }
.section h2 { margin-bottom: 16px; font-size: 16px; color: #333; }
table { width: 100%; border-collapse: collapse; font-size: 14px; }
th, td { padding: 10px 12px; text-align: left; border-bottom: 1px solid #eee; }
th { font-weight: 600; color: #666; font-size: 12px; text-transform: uppercase; }
tr:hover { background: #f9f9f9; }
.badge { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 11px; font-weight: 600; }
.badge-stream { background: #dbeafe; color: #2563eb; }
.badge-sync { background: #f3e8ff; color: #9333ea; }
.model-list { display: flex; flex-wrap: wrap; gap: 8px; }
.model-item { background: #f3f4f6; padding: 8px 12px; border-radius: 6px; font-size: 13px; }
.model-item .count { font-weight: 700; color: #333; }
.empty { text-align: center; padding: 40px; color: #999; }
</style>
</head>
<body>
<div class="container">
<h1>Token Usage Dashboard</h1>
<div class="toolbar">
  <button onclick="setRange('1h')">1 Hour</button>
  <button onclick="setRange('24h')">24 Hours</button>
  <button onclick="setRange('7d')" class="active">7 Days</button>
  <button onclick="setRange('30d')">30 Days</button>
  <button onclick="setRange('')">All</button>
</div>
<div class="cards">
  <div class="card"><div class="label">Total Requests</div><div class="value" id="total-requests">-</div></div>
  <div class="card"><div class="label">Input Tokens</div><div class="value input" id="total-input">-</div></div>
  <div class="card"><div class="label">Output Tokens</div><div class="value output" id="total-output">-</div></div>
  <div class="card"><div class="label">Total Tokens</div><div class="value total" id="total-tokens">-</div></div>
  <div class="card"><div class="label">Avg Duration</div><div class="value" id="avg-duration">-</div></div>
</div>

<div class="section">
  <h2>Model Breakdown</h2>
  <div class="model-list" id="model-breakdown"></div>
</div>

<div class="section">
  <h2>Recent Requests</h2>
  <table>
    <thead><tr><th>Time</th><th>Model</th><th>Type</th><th>Input</th><th>Output</th><th>Total</th><th>Duration</th></tr></thead>
    <tbody id="records-body"></tbody>
  </table>
  <div class="empty" id="empty-msg">No usage data yet</div>
</div>
</div>

<script>
let currentRange = '7d';
function fmt(n) { return n.toLocaleString(); }

function setRange(r) {
  currentRange = r;
  document.querySelectorAll('.toolbar button').forEach(b => b.classList.remove('active'));
  event.target.classList.add('active');
  loadData();
}

function loadData() {
  const url = '/dashboard/api/usage' + (currentRange ? '?range=' + currentRange : '');
  fetch(url)
    .then(r => r.json())
    .then(data => {
      document.getElementById('total-requests').textContent = fmt(data.total_requests);
      document.getElementById('total-input').textContent = fmt(data.total_input_tokens);
      document.getElementById('total-output').textContent = fmt(data.total_output_tokens);
      document.getElementById('total-tokens').textContent = fmt(data.total_tokens);
      document.getElementById('avg-duration').textContent = data.avg_duration_ms + 'ms';

      const modelDiv = document.getElementById('model-breakdown');
      modelDiv.innerHTML = '';
      for (const [model, count] of Object.entries(data.model_breakdown || {})) {
        modelDiv.innerHTML += '<div class="model-item"><span class="count">' + count + '</span> ' + model + '</div>';
      }

      const tbody = document.getElementById('records-body');
      const emptyMsg = document.getElementById('empty-msg');
      if (!data.records || data.records.length === 0) {
        tbody.innerHTML = '';
        emptyMsg.style.display = 'block';
        return;
      }
      emptyMsg.style.display = 'none';

      const rows = data.records.slice().reverse().slice(0, 200).map(r => {
        const t = new Date(r.timestamp);
        const time = t.toLocaleString();
        const badge = r.stream ? '<span class="badge badge-stream">stream</span>' : '<span class="badge badge-sync">sync</span>';
        return '<tr><td>' + time + '</td><td>' + r.model + '</td><td>' + badge + '</td><td>' + fmt(r.input_tokens) + '</td><td>' + fmt(r.output_tokens) + '</td><td>' + fmt(r.total_tokens) + '</td><td>' + r.duration_ms + 'ms</td></tr>';
      });
      tbody.innerHTML = rows.join('');
    });
}
loadData();
setInterval(loadData, 10000);
</script>
</body>
</html>`
