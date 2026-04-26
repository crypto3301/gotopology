package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Server struct {
	net     *Network
	addr    string
	clients map[*websocket.Conn]bool
	mu      sync.Mutex
}

func NewServer(net *Network, addr string) *Server {
	return &Server{
		net:     net,
		addr:    addr,
		clients: make(map[*websocket.Conn]bool),
	}
}

func (s *Server) ListenAndServe() error {
	go s.broadcastEvents()

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.serveFrontend)
	mux.HandleFunc("/ws", s.handleWS)
	mux.HandleFunc("/api/route", s.handleRoute)
	mux.HandleFunc("/api/reset", s.handleReset)

	log.Printf("DSR Симуляция запущена → http://localhost%s", s.addr)
	return http.ListenAndServe(s.addr, mux)
}

func (s *Server) broadcastEvents() {
	for evt := range s.net.Events() {
		data, err := json.Marshal(evt)
		if err != nil {
			continue
		}
		s.mu.Lock()
		for conn := range s.clients {
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				conn.Close()
				delete(s.clients, conn)
			}
		}
		s.mu.Unlock()
	}
}

func (s *Server) serveFrontend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, Page)
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}

	s.mu.Lock()
	s.clients[conn] = true
	s.mu.Unlock()

	topo := s.net.GetTopologyEvent()
	if data, err := json.Marshal(topo); err == nil {
		conn.WriteMessage(websocket.TextMessage, data)
	}

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	s.mu.Lock()
	delete(s.clients, conn)
	s.mu.Unlock()
	conn.Close()
}

func (s *Server) handleRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Source      int `json:"source"`
		Destination int `json:"destination"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if err := s.net.InitiateRoute(req.Source, req.Destination); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error":"%s"}`, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status":"ok"}`)
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.net.Reset()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status":"reset"}`)
}

const Page = `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<title>DSR — Dynamic Source Routing</title>
<link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;600&family=IBM+Plex+Sans:wght@300;400;600&display=swap" rel="stylesheet">
<style>
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

:root {
  --black:  #0a0a0a;
  --white:  #ffffff;
  --grey1:  #f4f4f4;
  --grey2:  #e0e0e0;
  --grey3:  #a0a0a0;
  --grey4:  #606060;
  --mono:   'IBM Plex Mono', monospace;
  --sans:   'IBM Plex Sans', sans-serif;
}

html, body { height: 100%; overflow: hidden; background: var(--white); color: var(--black); font-family: var(--sans); font-size: 13px; }
body { display: flex; }

#sidebar { width: 280px; min-width: 280px; height: 100vh; border-right: 1px solid var(--grey2); display: flex; flex-direction: column; background: var(--white); }
#canvas-container { flex: 1; position: relative; background: var(--grey1); overflow: hidden; }
canvas { display: block; }

.sidebar-header { padding: 28px 24px 20px; border-bottom: 1px solid var(--grey2); }
.sidebar-header .label { font-family: var(--mono); font-size: 10px; font-weight: 600; letter-spacing: .12em; text-transform: uppercase; color: var(--grey3); margin-bottom: 6px; }
.sidebar-header h1 { font-size: 15px; font-weight: 600; letter-spacing: -.01em; line-height: 1.3; }

.sidebar-section { padding: 20px 24px; border-bottom: 1px solid var(--grey2); display: flex; flex-direction: column; gap: 10px; }
.section-label { font-family: var(--mono); font-size: 10px; font-weight: 600; letter-spacing: .12em; text-transform: uppercase; color: var(--grey3); }

.field { display: flex; flex-direction: column; gap: 4px; }
.field-label { font-size: 11px; color: var(--grey4); }

select {
  appearance: none; -webkit-appearance: none; width: 100%;
  padding: 9px 32px 9px 12px;
  background: var(--grey1) url("data:image/svg+xml,%3Csvg width='10' height='6' viewBox='0 0 10 6' xmlns='http://www.w3.org/2000/svg'%3E%3Cpath d='M0 0l5 6 5-6z' fill='%23606060'/%3E%3C/svg%3E") no-repeat right 12px center;
  border: 1px solid var(--grey2); border-radius: 0;
  color: var(--black); font-family: var(--mono); font-size: 12px;
  cursor: pointer; outline: none; transition: border-color .15s;
}
select:focus { border-color: var(--black); }
select:disabled { opacity: .4; cursor: not-allowed; }

.btn {
  width: 100%; padding: 10px 14px;
  border: 1px solid var(--black); background: var(--black); color: var(--white);
  font-family: var(--mono); font-size: 11px; font-weight: 600;
  letter-spacing: .06em; text-transform: uppercase;
  cursor: pointer; transition: background .15s, color .15s; border-radius: 0;
}
.btn:hover:not(:disabled) { background: var(--white); color: var(--black); }
.btn:disabled { opacity: .35; cursor: not-allowed; }
.btn-ghost { background: var(--white); color: var(--black); }
.btn-ghost:hover:not(:disabled) { background: var(--grey1); }

.log-section { flex: 1; display: flex; flex-direction: column; overflow: hidden; padding: 16px 24px; gap: 8px; }
#log { flex: 1; overflow-y: auto; font-family: var(--mono); font-size: 11px; line-height: 1.85; color: var(--grey4); scrollbar-width: thin; scrollbar-color: var(--grey2) transparent; }
.log-line   { border-bottom: 1px solid var(--grey1); padding: 1px 0; }
.log-line:last-child { border-bottom: none; }
.log-err    { color: #b00020 !important; }
.log-warn   { color: #996600 !important; }
.log-rreq   { color: #c2410c !important; }
.log-rrep   { color: #1d4ed8 !important; }
.log-found  { color: #15803d !important; font-weight: 600; }
.log-cache  { color: #7e22ce !important; }
.log-dup    { color: #9ca3af !important; }

.stats-bar { display: grid; grid-template-columns: 1fr 1fr; border-top: 1px solid var(--grey2); }
.stat { padding: 14px 24px; border-right: 1px solid var(--grey2); }
.stat:last-child { border-right: none; }
.stat-value { font-family: var(--mono); font-size: 20px; font-weight: 600; line-height: 1; }
.stat-label { font-size: 10px; color: var(--grey3); text-transform: uppercase; letter-spacing: .08em; margin-top: 4px; }

#status {
  position: absolute; top: 20px; left: 50%; transform: translateX(-50%);
  background: var(--white); color: var(--black);
  padding: 7px 18px; font-family: var(--mono); font-size: 11px;
  border: 1px solid var(--grey2); white-space: nowrap;
  pointer-events: none; letter-spacing: .04em;
}

#tooltip {
  position: absolute; background: var(--black); color: var(--white);
  font-family: var(--mono); font-size: 11px; padding: 6px 12px;
  pointer-events: none; opacity: 0; transition: opacity .1s;
  z-index: 10; white-space: nowrap;
}
</style>
</head>
<body>

<div id="sidebar">
  <div class="sidebar-header">
    <div class="label">Протокол маршрутизации</div>
    <h1>Dynamic Source Routing</h1>
  </div>

  <div class="sidebar-section">
    <div class="section-label">Маршрут</div>
    <div class="field">
      <div class="field-label">Источник</div>
      <select id="source" disabled></select>
    </div>
    <div class="field">
      <div class="field-label">Назначение</div>
      <select id="destination" disabled></select>
    </div>
  </div>

  <div class="sidebar-section">
    <button class="btn" id="btn-route" disabled onclick="startRoute()">Найти маршрут</button>
    <button class="btn btn-ghost" onclick="resetSim()">Сбросить</button>
  </div>

  <div class="log-section">
    <div class="section-label">Журнал событий</div>
    <div id="log">
      <div class="log-line" style="color:var(--grey3)">Ожидание подключения...</div>
    </div>
  </div>

  <div class="stats-bar">
    <div class="stat">
      <div class="stat-value" id="stat-nodes">—</div>
      <div class="stat-label">Узлов</div>
    </div>
    <div class="stat">
      <div class="stat-value" id="stat-edges">—</div>
      <div class="stat-label">Рёбер</div>
    </div>
  </div>
</div>

<div id="canvas-container">
  <div id="status">Подключение к серверу...</div>
  <div id="tooltip"></div>
  <canvas id="canvas"></canvas>
</div>

<script>
var canvas      = document.getElementById('canvas');
var ctx         = canvas.getContext('2d');
var tooltip     = document.getElementById('tooltip');
var nodes       = [];
var edges       = [];
var positions   = {};
var hoveredNode = null;
var edgeColors  = {};   // временные цвета (мигание RREQ/RREP)
var routeEdges  = {};   // постоянные цвета найденного маршрута

// ── WebSocket ──────────────────────────────────────────────────────────────
var ws = new WebSocket('ws://' + location.host + '/ws');

ws.onopen = function() {
  addLog('WebSocket подключён');
  setStatus('Ожидание топологии...');
};

ws.onmessage = function(ev) {
  var data;
  try { data = JSON.parse(ev.data); } catch(e) {
    addLog('[ошибка] Разбор JSON: ' + e, 'err');
    return;
  }

  console.log('[DSR] событие:', data);

  // ── Топология ──────────────────────────────────────────────────────────
  if (data.type === 'topology') {
    var rawNodes = data.nodes || data.Nodes || [];
    var rawEdges = data.edges || data.Edges || [];

    nodes = rawNodes.map(function(n, i) {
      if (typeof n === 'number' || typeof n === 'string') return { id: n };
      return { id: (n.id !== undefined) ? n.id : (n.ID !== undefined) ? n.ID : i };
    });

    edges = rawEdges.map(function(e) {
      if (Array.isArray(e)) return { from: e[0], to: e[1] };
      return {
        from: (e.from !== undefined) ? e.from : e.From,
        to:   (e.to   !== undefined) ? e.to   : e.To
      };
    });

    resizeCanvas();
    layoutNodes();
    draw();
    fillSelects();

    document.getElementById('stat-nodes').textContent = nodes.length;
    document.getElementById('stat-edges').textContent = edges.length;
    document.getElementById('btn-route').disabled = false;

    setStatus('Топология загружена — ' + nodes.length + ' узлов, ' + edges.length + ' рёбер');
    addLog('Получено: ' + nodes.length + ' узлов, ' + edges.length + ' рёбер');
    return;
  }

  // ── RREQ ───────────────────────────────────────────────────────────────
  if (data.type === 'rreq') {
    flashEdge(data.from, data.to, '#f97316', 600);
    addLog('RREQ [' + data.rreqId + ']: ' + data.from + ' → ' + data.to +
           ' | путь: [' + (data.routeRecord || []).join('→') + ']', 'rreq');
    setStatus('RREQ: ' + data.from + ' → ' + data.to);
    return;
  }

  // ── RREP ───────────────────────────────────────────────────────────────
  if (data.type === 'rrep') {
    flashEdge(data.from, data.to, '#3b82f6', 600);
    addLog('RREP: ' + data.from + ' → ' + data.to +
           ' | маршрут: [' + (data.route || []).join('→') + ']', 'rrep');
    setStatus('RREP: ' + data.from + ' → ' + data.to);
    return;
  }

  // ── Маршрут найден ─────────────────────────────────────────────────────
  if (data.type === 'route_found') {
    paintRoute(data.route, '#16a34a');
    addLog('✓ МАРШРУТ: ' + (data.route || []).join(' → '), 'found');
    setStatus('Маршрут найден: ' + (data.route || []).join(' → '));
    return;
  }

  // ── Кэш ────────────────────────────────────────────────────────────────
  if (data.type === 'cache_hit') {
    paintRoute(data.route, '#9333ea');
    addLog('⚡ Кэш узла ' + data.node + ': [' + (data.route || []).join('→') + ']', 'cache');
    setStatus('Маршрут из кэша!');
    return;
  }

  // ── Дубликат RREQ ──────────────────────────────────────────────────────
  if (data.type === 'rreq_seen') {
    addLog('⊘ Узел ' + data.node + ': дубль RREQ [' + data.rreqId + ']', 'dup');
    return;
  }

  // ── Лог ────────────────────────────────────────────────────────────────
  if (data.type === 'log') {
    addLog(data.message);
    return;
  }

  // ── Сброс ──────────────────────────────────────────────────────────────
  if (data.type === 'reset') {
    edgeColors = {};
    routeEdges = {};
    draw();
    addLog('Симуляция сброшена');
    setStatus('Сброс выполнен');
    return;
  }
};

ws.onerror = function() {
  addLog('[ошибка] WebSocket: ошибка соединения', 'err');
  setStatus('Ошибка WebSocket');
};

ws.onclose = function() {
  setStatus('Соединение закрыто');
  addLog('WebSocket закрыт');
};

// ── Canvas resize ─────────────────────────────────────────────────────────
function resizeCanvas() {
  var p = canvas.parentElement;
  canvas.width  = p.clientWidth;
  canvas.height = p.clientHeight;
}

window.addEventListener('resize', function() {
  resizeCanvas();
  if (nodes.length) { layoutNodes(); draw(); }
});

// ── Layout ────────────────────────────────────────────────────────────────
function layoutNodes() {
  var w = canvas.width, h = canvas.height;
  var cx = w / 2, cy = h / 2;
  var rx = Math.min(w, h) * 0.36;
  var ry = rx * 0.78;

  nodes.forEach(function(node, i) {
    var angle = (i / nodes.length) * Math.PI * 2 - Math.PI / 2;
    positions[node.id] = {
      x: cx + Math.cos(angle) * rx,
      y: cy + Math.sin(angle) * ry
    };
  });
}

// ── Draw ──────────────────────────────────────────────────────────────────
function edgeKey(u, v) {
  return Math.min(u, v) + '-' + Math.max(u, v);
}

function draw() {
  ctx.clearRect(0, 0, canvas.width, canvas.height);

  // Рёбра
  edges.forEach(function(e) {
    var a = positions[e.from];
    var b = positions[e.to];
    if (!a || !b) return;
    var key = edgeKey(e.from, e.to);
    var col = edgeColors[key] || routeEdges[key] || '#c8c8c8';
    var w   = (edgeColors[key] || routeEdges[key]) ? 3.5 : 1.5;
    ctx.strokeStyle = col;
    ctx.lineWidth   = w;
    ctx.beginPath();
    ctx.moveTo(a.x, a.y);
    ctx.lineTo(b.x, b.y);
    ctx.stroke();
  });

  // Узлы
  nodes.forEach(function(node) {
    var p = positions[node.id];
    if (!p) return;
    var isHov = (hoveredNode === node.id);
    var r     = isHov ? 18 : 14;

    if (isHov) { ctx.shadowColor = 'rgba(0,0,0,.14)'; ctx.shadowBlur = 14; }

    ctx.fillStyle   = isHov ? '#0a0a0a' : '#ffffff';
    ctx.strokeStyle = '#0a0a0a';
    ctx.lineWidth   = isHov ? 2 : 1;
    ctx.beginPath();
    ctx.arc(p.x, p.y, r, 0, Math.PI * 2);
    ctx.fill();
    ctx.stroke();
    ctx.shadowBlur = 0;

    ctx.fillStyle    = isHov ? '#ffffff' : '#0a0a0a';
    ctx.font         = 'bold 10px "IBM Plex Mono", monospace';
    ctx.textAlign    = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(String(node.id), p.x, p.y);
  });
}

// ── Edge animations ───────────────────────────────────────────────────────
function flashEdge(u, v, color, duration) {
  var key = edgeKey(u, v);
  edgeColors[key] = color;
  draw();
  setTimeout(function() {
    delete edgeColors[key];
    draw();
  }, duration || 600);
}

function paintRoute(route, color) {
  routeEdges = {};
  if (!route) return;
  for (var i = 0; i < route.length - 1; i++) {
    routeEdges[edgeKey(route[i], route[i + 1])] = color;
  }
  draw();
}

// ── Hover ─────────────────────────────────────────────────────────────────
canvas.addEventListener('mousemove', function(ev) {
  if (!nodes.length) return;
  var rect = canvas.getBoundingClientRect();
  var mx = ev.clientX - rect.left;
  var my = ev.clientY - rect.top;
  var found = null, best = 500;

  nodes.forEach(function(node) {
    var p = positions[node.id];
    if (!p) return;
    var d = (p.x - mx) * (p.x - mx) + (p.y - my) * (p.y - my);
    if (d < best) { best = d; found = node; }
  });

  var hit = found && best < 400;
  if (hit) {
    if (hoveredNode !== found.id) { hoveredNode = found.id; draw(); }
    tooltip.style.opacity = '1';
    tooltip.style.left    = (ev.clientX - rect.left + 16) + 'px';
    tooltip.style.top     = (ev.clientY - rect.top  - 12) + 'px';
    tooltip.textContent   = 'Узел ' + found.id;
  } else {
    if (hoveredNode !== null) { hoveredNode = null; draw(); }
    tooltip.style.opacity = '0';
  }
});

canvas.addEventListener('mouseleave', function() {
  hoveredNode = null;
  tooltip.style.opacity = '0';
  draw();
});

// ── Selects ───────────────────────────────────────────────────────────────
function fillSelects() {
  var src = document.getElementById('source');
  var dst = document.getElementById('destination');
  src.innerHTML = dst.innerHTML = '';
  nodes.forEach(function(n) {
    src.add(new Option('Узел ' + n.id, n.id));
    dst.add(new Option('Узел ' + n.id, n.id));
  });
  src.disabled = dst.disabled = false;
  if (nodes.length > 1) dst.selectedIndex = nodes.length - 1;
}

// ── Actions ───────────────────────────────────────────────────────────────
function startRoute() {
  var src = parseInt(document.getElementById('source').value);
  var dst = parseInt(document.getElementById('destination').value);
  if (src === dst) {
    addLog('[предупреждение] Источник и назначение совпадают', 'warn');
    return;
  }
  routeEdges = {};
  edgeColors  = {};
  draw();
  addLog('Поиск маршрута: ' + src + ' → ' + dst);
  setStatus('Поиск маршрута ' + src + ' → ' + dst + '...');
  fetch('/api/route', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ source: src, destination: dst })
  }).then(function(r) { return r.json(); })
    .then(function(d) { if (d.error) addLog('[ошибка] ' + d.error, 'err'); })
    .catch(function(e) { addLog('[ошибка] /api/route: ' + e, 'err'); });
}

function resetSim() {
  fetch('/api/reset', { method: 'POST' })
    .catch(function(e) { addLog('[ошибка] /api/reset: ' + e, 'err'); });
  document.getElementById('log').innerHTML = '';
  edgeColors = {};
  routeEdges = {};
  draw();
  addLog('Симуляция сброшена');
  setStatus('Сброс выполнен');
}

// ── Helpers ───────────────────────────────────────────────────────────────
function setStatus(txt) { document.getElementById('status').textContent = txt; }

function addLog(msg, type) {
  var log  = document.getElementById('log');
  var line = document.createElement('div');
  line.className   = 'log-line' + (type ? ' log-' + type : '');
  line.textContent = msg;
  log.appendChild(line);
  log.scrollTop = log.scrollHeight;
}

window.addEventListener('load', function() { resizeCanvas(); });
</script>
</body>
</html>`
