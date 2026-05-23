package dashboard

import (
	"encoding/json"
	"net/http"
	"strconv"
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
	var err error

	now := time.Now()
	switch rangeParam {
	case "1h":
		records, err = h.store.Since(now.Add(-1 * time.Hour))
	case "24h":
		records, err = h.store.Since(now.Add(-24 * time.Hour))
	case "7d":
		records, err = h.store.Since(now.Add(-7 * 24 * time.Hour))
	case "30d":
		records, err = h.store.Since(now.Add(-30 * 24 * time.Hour))
	default:
		records, err = h.store.Records()
	}
	if err != nil {
		http.Error(w, `{"error":"internal storage error"}`, http.StatusInternalServerError)
		return
	}

	page := parseIntParam(r, "page", 1)
	pageSize := parseIntParam(r, "page_size", 100)
	if pageSize > 1000 {
		pageSize = 1000
	}
	if pageSize < 1 {
		pageSize = 1
	}
	if page < 1 {
		page = 1
	}

	type summary struct {
		TotalRequests  int                   `json:"total_requests"`
		TotalInput     int                   `json:"total_input_tokens"`
		TotalOutput    int                   `json:"total_output_tokens"`
		TotalTokens    int                   `json:"total_tokens"`
		AvgDuration    int64                 `json:"avg_duration_ms"`
		ModelBreakdown map[string]int        `json:"model_breakdown"`
		Records        []storage.UsageRecord `json:"records"`
		Page           int                   `json:"page"`
		PageSize       int                   `json:"page_size"`
		TotalRecords   int                   `json:"total_records"`
		TotalPages     int                   `json:"total_pages"`
	}

	s := summary{
		ModelBreakdown: make(map[string]int),
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

	total := len(records)
	s.TotalRecords = total
	s.Page = page
	s.PageSize = pageSize
	s.TotalPages = (total + pageSize - 1) / pageSize

	start := (page - 1) * pageSize
	end := start + pageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	s.Records = records[start:end]
	if s.Records == nil {
		s.Records = []storage.UsageRecord{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

func parseIntParam(r *http.Request, name string, defaultVal int) int {
	s := r.URL.Query().Get(name)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
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
.pagination { display: flex; align-items: center; justify-content: center; gap: 12px; margin-top: 16px; font-size: 13px; }
.pagination button { padding: 6px 12px; border: 1px solid #ddd; background: #fff; border-radius: 4px; cursor: pointer; }
.pagination button:disabled { opacity: 0.4; cursor: default; }
.pagination .page-info { color: #666; }
.chart-container { position: relative; }
.chart { display: flex; align-items: flex-end; gap: 2px; height: 180px; padding: 0 4px; border-bottom: 1px solid #e5e7eb; }
.chart-bar { flex: 1; min-width: 4px; display: flex; flex-direction: column; justify-content: flex-end; position: relative; cursor: pointer; }
.chart-bar .bar-input { background: #2563eb; border-radius: 2px 2px 0 0; }
.chart-bar .bar-output { background: #16a34a; border-radius: 2px 2px 0 0; }
.chart-bar:hover .bar-tooltip { display: block; }
.bar-tooltip { display: none; position: absolute; bottom: 100%; left: 50%; transform: translateX(-50%); background: #333; color: #fff; padding: 4px 8px; border-radius: 4px; font-size: 11px; white-space: nowrap; z-index: 10; }
.chart-legend { display: flex; gap: 16px; margin-top: 8px; font-size: 12px; color: #666; }
.legend-item { display: flex; align-items: center; gap: 4px; }
.legend-color { width: 12px; height: 12px; border-radius: 2px; }
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
  <h2>Token Usage Over Time</h2>
  <div class="chart-container" id="chart-container">
    <div class="chart" id="chart"></div>
    <div class="chart-legend">
      <span class="legend-item"><span class="legend-color" style="background:#2563eb"></span>Input</span>
      <span class="legend-item"><span class="legend-color" style="background:#16a34a"></span>Output</span>
    </div>
  </div>
</div>

<div class="section">
  <h2>Model Breakdown</h2>
  <div class="model-list" id="model-breakdown"></div>
</div>

<div class="section">
  <h2>Recent Requests</h2>
  <table>
    <thead><tr><th>Time</th><th>Model</th><th>Type</th><th>Input</th><th>Output</th><th>Total</th><th>Duration</th><th>Preview</th></tr></thead>
    <tbody id="records-body"></tbody>
  </table>
  <div class="empty" id="empty-msg">No usage data yet</div>
  <div class="pagination" id="pagination"></div>
</div>
</div>

<script>
let currentRange = '7d';
let currentPage = 1;
const pageSize = 50;
function fmt(n) { return n.toLocaleString(); }
function esc(s) { const d = document.createElement('div'); d.textContent = s; return d.innerHTML; }

function setRange(r) {
  currentRange = r;
  currentPage = 1;
  document.querySelectorAll('.toolbar button').forEach(b => b.classList.remove('active'));
  event.target.classList.add('active');
  loadData();
}

function goToPage(p) { currentPage = p; loadData(); }

function loadData() {
  let url = '/dashboard/api/usage?page=' + currentPage + '&page_size=' + pageSize;
  if (currentRange) url += '&range=' + currentRange;
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
        modelDiv.innerHTML += '<div class="model-item"><span class="count">' + count + '</span> ' + esc(model) + '</div>';
      }

      renderChart(data.records || []);

      const tbody = document.getElementById('records-body');
      const emptyMsg = document.getElementById('empty-msg');
      if (!data.records || data.records.length === 0) {
        tbody.innerHTML = '';
        emptyMsg.style.display = 'block';
        return;
      }
      emptyMsg.style.display = 'none';

      const rows = data.records.map(r => {
        const t = new Date(r.timestamp);
        const time = t.toLocaleString();
        const badge = r.stream ? '<span class="badge badge-stream">stream</span>' : '<span class="badge badge-sync">sync</span>';
        return '<tr><td>' + esc(time) + '</td><td>' + esc(r.model) + '</td><td>' + badge + '</td><td>' + fmt(r.input_tokens) + '</td><td>' + fmt(r.output_tokens) + '</td><td>' + fmt(r.total_tokens) + '</td><td>' + r.duration_ms + 'ms</td><td>' + esc(r.input_preview || '') + '</td></tr>';
      });
      tbody.innerHTML = rows.join('');
      renderPagination(data.page, data.total_pages, data.total_records);
    });
}
function renderPagination(page, totalPages, totalRecords) {
  const div = document.getElementById('pagination');
  if (totalPages <= 1) { div.innerHTML = ''; return; }
  let html = '<button ' + (page <= 1 ? 'disabled' : 'onclick="goToPage(' + (page-1) + ')"') + '>&laquo; Prev</button>';
  html += '<span class="page-info">Page ' + page + ' / ' + totalPages + ' (' + fmt(totalRecords) + ' records)</span>';
  html += '<button ' + (page >= totalPages ? 'disabled' : 'onclick="goToPage(' + (page+1) + ')"') + '>Next &raquo;</button>';
  div.innerHTML = html;
}
function renderChart(records) {
  const chart = document.getElementById('chart');
  if (!records.length) { chart.innerHTML = '<div class="empty">No data for chart</div>'; return; }

  const bucketCount = 24;
  const times = records.map(r => new Date(r.timestamp).getTime());
  const minT = Math.min(...times), maxT = Math.max(...times);
  const span = Math.max(maxT - minT, 1);
  const bucketSize = span / bucketCount;

  const buckets = Array.from({length: bucketCount}, () => ({input: 0, output: 0}));
  records.forEach(r => {
    const t = new Date(r.timestamp).getTime();
    const idx = Math.min(Math.floor((t - minT) / bucketSize), bucketCount - 1);
    buckets[idx].input += r.input_tokens;
    buckets[idx].output += r.output_tokens;
  });

  const maxVal = Math.max(...buckets.map(b => b.input + b.output), 1);
  const chartHeight = 160;

  chart.innerHTML = buckets.map((b, i) => {
    const inH = (b.input / maxVal) * chartHeight;
    const outH = (b.output / maxVal) * chartHeight;
    const t = new Date(minT + i * bucketSize);
    const label = t.toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'});
    const total = fmt(b.input + b.output);
    return '<div class="chart-bar">' +
      '<div class="bar-tooltip">' + esc(label) + ' | ' + total + ' tokens</div>' +
      '<div class="bar-output" style="height:' + outH + 'px"></div>' +
      '<div class="bar-input" style="height:' + inH + 'px"></div>' +
      '</div>';
  }).join('');
}

loadData();
setInterval(loadData, 10000);
</script>
</body>
</html>`
