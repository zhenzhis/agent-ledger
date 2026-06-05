const $ = (id) => document.getElementById(id);

const fmt = (value) => {
  const n = Number(value || 0);
  if (Math.abs(n) >= 1e9) return `${(n / 1e9).toFixed(1)}B`;
  if (Math.abs(n) >= 1e6) return `${(n / 1e6).toFixed(1)}M`;
  if (Math.abs(n) >= 1e3) return `${(n / 1e3).toFixed(1)}K`;
  return String(Math.round(n));
};

const fmtCost = (value) => {
  const n = Number(value || 0);
  return n >= 1 ? `$${n.toFixed(2)}` : `$${n.toFixed(4)}`;
};

function localDateStr(d) {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

const I18N = {
  en: {
    from: "From",
    to: "To",
    granularity: "Granularity",
    source: "Source",
    model: "Model",
    project: "Project",
    branch: "Branch",
    time: "Time",
    tokens: "Tokens",
    cost: "Cost",
    refresh: "Refresh",
    totalTokens: "Total Tokens",
    totalCost: "Total Cost",
    sessions: "Sessions",
    prompts: "Prompts",
    activityMatrix: "Activity Matrix",
    tokenThroughput: "Token Throughput",
    costOverTime: "Cost Trend",
    modelAllocation: "Model Allocation",
    sessionLedger: "Session Ledger",
    filterProject: "Filter project, path, or branch...",
    today: "Today",
    thisWeek: "This Week",
    thisMonth: "This Month",
    thisYear: "This Year",
    last3d: "Last 3D",
    last7d: "Last 7D",
    last30d: "Last 30D",
    custom: "Custom",
    light: "Light",
    dark: "Dark",
    system: "System",
    autoOn: "Auto On",
    autoOff: "Auto",
    input: "Input",
    output: "Output",
    cacheRead: "Cache Read",
    cacheCreate: "Cache Write",
    gran_1m: "1m",
    gran_30m: "30m",
    gran_1h: "1h",
    gran_6h: "6h",
    gran_12h: "12h",
    gran_1d: "1d",
    gran_1w: "1w",
    gran_1M: "1M",
    allSources: "All Sources",
    allModels: "All Models",
    claudeCode: "Claude Code",
    codex: "Codex",
    openClaw: "OpenClaw",
    openCode: "OpenCode",
    kiro: "kiro",
    pi: "Pi",
    calls: "calls",
    cache: "cache",
    perCall: "per call",
    tokensPerCall: "tokens/call",
    noData: "No data in range",
    loadingDetails: "Loading details...",
    noDetails: "No detailed model breakdown.",
    detailFailed: "Failed to load details.",
    justNow: "just now",
    minAgo: "m ago",
    hourAgo: "h ago",
    dayAgo: "d ago",
    total: "total",
    of: "of",
    updated: "Updated",
    refreshFailed: "Refresh failed",
    unknownModel: "Unknown model",
    rows: "rows",
    unitMin: "min",
    unitSec: "sec",
  },
  zh: {
    from: "起始",
    to: "结束",
    granularity: "粒度",
    source: "来源",
    model: "模型",
    project: "项目",
    branch: "分支",
    time: "时间",
    tokens: "Tokens",
    cost: "费用",
    refresh: "刷新",
    totalTokens: "总 Tokens",
    totalCost: "总费用",
    sessions: "会话数",
    prompts: "Prompt 数",
    activityMatrix: "活动矩阵",
    tokenThroughput: "Token 吞吐",
    costOverTime: "费用趋势",
    modelAllocation: "模型分布",
    sessionLedger: "会话账本",
    filterProject: "筛选项目、路径或分支...",
    today: "今天",
    thisWeek: "本周",
    thisMonth: "本月",
    thisYear: "今年",
    last3d: "近 3 天",
    last7d: "近 7 天",
    last30d: "近 30 天",
    custom: "自定义",
    light: "浅色",
    dark: "深色",
    system: "跟随系统",
    autoOn: "自动开启",
    autoOff: "自动",
    input: "输入",
    output: "输出",
    cacheRead: "缓存读",
    cacheCreate: "缓存写",
    gran_1m: "1 分钟",
    gran_30m: "30 分钟",
    gran_1h: "1 小时",
    gran_6h: "6 小时",
    gran_12h: "12 小时",
    gran_1d: "1 天",
    gran_1w: "1 周",
    gran_1M: "1 月",
    allSources: "全部来源",
    allModels: "全部模型",
    claudeCode: "Claude Code",
    codex: "Codex",
    openClaw: "OpenClaw",
    openCode: "OpenCode",
    kiro: "kiro",
    pi: "Pi",
    calls: "次调用",
    cache: "缓存",
    perCall: "每次调用",
    tokensPerCall: "tokens/调用",
    noData: "当前区间暂无数据",
    loadingDetails: "正在加载明细...",
    noDetails: "暂无模型明细。",
    detailFailed: "明细加载失败。",
    justNow: "刚刚",
    minAgo: "分钟前",
    hourAgo: "小时前",
    dayAgo: "天前",
    total: "总计",
    of: "/",
    updated: "已更新",
    refreshFailed: "刷新失败",
    unknownModel: "未知模型",
    rows: "行",
    unitMin: "分钟",
    unitSec: "秒",
  },
};

const PRESETS = ["today", "thisWeek", "thisMonth", "thisYear", "last3d", "last7d", "last30d", "custom"];
const GRANULARITIES = ["1m", "30m", "1h", "6h", "12h", "1d", "1w", "1M"];
const REFRESH_INTERVALS = [30, 60, 300, 1800, 3600];
const COLORS = ["#2dd4bf", "#60a5fa", "#22c55e", "#f59e0b", "#a78bfa", "#fb7185", "#38bdf8", "#f97316", "#84cc16", "#e879f9"];
const SOURCES = [
  ["", "allSources"],
  ["claude", "claudeCode"],
  ["codex", "codex"],
  ["openclaw", "openClaw"],
  ["opencode", "openCode"],
  ["kiro", "kiro"],
  ["pi", "pi"],
];
const KNOWN_SOURCES = new Set(SOURCES.map(([value]) => value).filter(Boolean));
const PAGE_SIZE = 20;

const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

let state = {
  lang: localStorage.getItem("au-lang") || (navigator.language.toLowerCase().startsWith("zh") ? "zh" : "en"),
  theme: localStorage.getItem("au-theme") || "dark",
  preset: localStorage.getItem("au-preset") || "today",
  granularity: localStorage.getItem("au-granularity") || "1h",
  autoRefresh: localStorage.getItem("au-autoRefresh") !== "false",
  refreshInterval: Number(localStorage.getItem("au-refreshInterval") || 300),
  customFrom: localStorage.getItem("au-customFrom") || "",
  customTo: localStorage.getItem("au-customTo") || "",
  source: localStorage.getItem("au-source") || "",
  model: localStorage.getItem("au-model") || "",
};

let charts = {};
let autoTimer = null;
let statusTimer = null;
let allSessions = [];
let sessionSort = { key: "start_time", dir: "desc" };
let sessionPage = 1;
let expandedSessions = new Set();
let sessionKeyToID = new Map();
let isFetching = false;

function t(key) {
  return (I18N[state.lang] || I18N.en)[key] || key;
}

function persist(key, value) {
  state[key] = value;
  localStorage.setItem(`au-${key}`, String(value));
}

function setText(id, value) {
  const el = $(id);
  if (el) el.textContent = value;
}

function fillSelect(select, options, selected) {
  const fragment = document.createDocumentFragment();
  options.forEach(({ value, label }) => {
    const opt = new Option(label, value, false, value === selected);
    fragment.appendChild(opt);
  });
  select.replaceChildren(fragment);
}

function applyTheme() {
  const systemDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
  const theme = state.theme === "system" ? (systemDark ? "dark" : "light") : state.theme;
  document.documentElement.dataset.theme = theme;
  document.documentElement.style.colorScheme = theme;
  Object.values(charts).forEach((chart) => chart && chart.resize());
}

function getThemeColors() {
  const cs = getComputedStyle(document.documentElement);
  return {
    bg: cs.getPropertyValue("--chart-bg").trim() || "transparent",
    text: cs.getPropertyValue("--chart-text").trim() || "#edf2f7",
    muted: cs.getPropertyValue("--chart-muted").trim() || "#83909e",
    grid: cs.getPropertyValue("--chart-grid").trim() || "#25303b",
    tooltipBg: cs.getPropertyValue("--tooltip-bg").trim() || "rgba(16,20,25,0.96)",
    tooltipBorder: cs.getPropertyValue("--tooltip-border").trim() || "#3a4654",
    accent: cs.getPropertyValue("--accent").trim() || "#2dd4bf",
    green: cs.getPropertyValue("--green").trim() || "#22c55e",
    amber: cs.getPropertyValue("--amber").trim() || "#f59e0b",
    blue: cs.getPropertyValue("--blue").trim() || "#60a5fa",
    purple: cs.getPropertyValue("--purple").trim() || "#a78bfa",
  };
}

function baseOpt() {
  const tc = getThemeColors();
  return {
    animation: !reducedMotion,
    backgroundColor: tc.bg,
    textStyle: {
      color: tc.text,
      fontFamily: "Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif",
    },
    grid: { left: 54, right: 18, top: 44, bottom: 38 },
    tooltip: {
      trigger: "axis",
      confine: true,
      renderMode: "richText",
      backgroundColor: tc.tooltipBg,
      borderColor: tc.tooltipBorder,
      textStyle: { color: tc.text },
      padding: [10, 12],
    },
  };
}

function emptyGraphic(text) {
  const tc = getThemeColors();
  return {
    type: "text",
    left: "center",
    top: "middle",
    style: {
      text,
      fill: tc.muted,
      fontSize: 13,
      fontFamily: "inherit",
    },
  };
}

function getTimeRange() {
  const now = new Date();
  const todayStr = localDateStr(now);
  switch (state.preset) {
    case "today":
      return { from: todayStr, to: todayStr };
    case "thisWeek": {
      const d = new Date(now);
      d.setDate(d.getDate() - ((d.getDay() + 6) % 7));
      return { from: localDateStr(d), to: todayStr };
    }
    case "thisMonth":
      return { from: `${todayStr.slice(0, 8)}01`, to: todayStr };
    case "thisYear":
      return { from: `${todayStr.slice(0, 5)}01-01`, to: todayStr };
    case "last3d": {
      const d = new Date(now);
      d.setDate(d.getDate() - 2);
      return { from: localDateStr(d), to: todayStr };
    }
    case "last7d": {
      const d = new Date(now);
      d.setDate(d.getDate() - 6);
      return { from: localDateStr(d), to: todayStr };
    }
    case "last30d": {
      const d = new Date(now);
      d.setDate(d.getDate() - 29);
      return { from: localDateStr(d), to: todayStr };
    }
    case "custom":
      return { from: state.customFrom || todayStr, to: state.customTo || todayStr };
    default:
      return { from: todayStr, to: todayStr };
  }
}

function updateRangeCaption() {
  const range = getTimeRange();
  setText("range-caption", `${range.from} ${t("to")} ${range.to} · ${t(`gran_${state.granularity}`)}`);
  $("custom-range-wrap").classList.toggle("is-hidden", state.preset !== "custom");
  $("from").value = state.customFrom || range.from;
  $("to").value = state.customTo || range.to;
}

async function api(path, opts = {}) {
  const range = getTimeRange();
  const params = new URLSearchParams({
    from: range.from,
    to: range.to,
    tz_offset: String(new Date().getTimezoneOffset()),
  });
  if (state.granularity) params.set("granularity", state.granularity);
  if (state.source) params.set("source", state.source);
  if (state.model && !opts.skipModel) params.set("model", state.model);

  const res = await fetch(`/api/${path}?${params.toString()}`);
  let body = null;
  try {
    body = await res.json();
  } catch (err) {
    if (res.ok) throw err;
  }
  if (!res.ok) {
    throw new Error((body && body.error) || `${path}: HTTP ${res.status}`);
  }
  return body;
}

function showStatus(message, kind = "info") {
  const el = $("status-line");
  el.textContent = message;
  el.classList.toggle("error", kind === "error");
  el.classList.add("show");
  if (statusTimer) clearTimeout(statusTimer);
  statusTimer = setTimeout(() => el.classList.remove("show"), kind === "error" ? 8000 : 2600);
}

function initCharts() {
  charts.activity = echarts.init($("chart-activity"));
  charts.tokens = echarts.init($("chart-tokens"));
  charts.cost = echarts.init($("chart-cost"));
  charts.models = echarts.init($("chart-models"));
  window.addEventListener("resize", () => Object.values(charts).forEach((chart) => chart && chart.resize()));
}

function buildModelColorMap(costModel) {
  const map = new Map();
  (costModel || []).forEach((row, index) => {
    const model = row.model || t("unknownModel");
    if (!map.has(model)) map.set(model, COLORS[index % COLORS.length]);
  });
  return map;
}

function renderStats(stats) {
  const totalTokens = Number(stats.total_tokens || 0);
  const totalCost = Number(stats.total_cost || 0);
  const calls = Number(stats.total_calls || 0);
  const cacheHit = Number(stats.cache_hit_rate || 0);

  setText("s-tokens", fmt(totalTokens));
  setText("s-cost", fmtCost(totalCost));
  setText("s-sessions", fmt(stats.total_sessions || 0));
  setText("s-prompts", fmt(stats.total_prompts || 0));
  setText("s-token-mix", `${fmt(calls ? totalTokens / calls : 0)} ${t("tokensPerCall")}`);
  setText("s-cost-per-call", `${fmtCost(calls ? totalCost / calls : 0)} ${t("perCall")}`);
  setText("s-calls", `${fmt(calls)} ${t("calls")}`);
  setText("s-cache-hit", `${(cacheHit * 100).toFixed(1)}% ${t("cache")}`);
}

function renderActivityMatrix(tokensTime) {
  const data = tokensTime || [];
  const labels = data.map((row) => row.date);
  const channels = [
    { key: "input_tokens", label: t("input") },
    { key: "output_tokens", label: t("output") },
    { key: "cache_read", label: t("cacheRead") },
    { key: "cache_create", label: t("cacheCreate") },
  ];
  const matrix = [];
  let max = 0;
  data.forEach((row, x) => {
    channels.forEach((channel, y) => {
      const value = Number(row[channel.key] || 0);
      max = Math.max(max, value);
      matrix.push([x, y, value]);
    });
  });
  const tc = getThemeColors();
  setText("activity-meta", `${labels.length} ${t("rows")}`);
  charts.activity.setOption({
    ...baseOpt(),
    grid: { left: 82, right: 18, top: 16, bottom: 42 },
    graphic: data.length === 0 ? emptyGraphic(t("noData")) : { type: "text", style: { text: "" } },
    tooltip: {
      ...baseOpt().tooltip,
      trigger: "item",
      formatter: (params) => {
        const [x, y, value] = params.data;
        return `${labels[x]}\n${channels[y].label}: ${fmt(value)}`;
      },
    },
    xAxis: {
      type: "category",
      data: labels,
      splitArea: { show: false },
      axisLine: { lineStyle: { color: tc.grid } },
      axisTick: { show: false },
      axisLabel: { color: tc.muted, hideOverlap: true, fontSize: 11 },
    },
    yAxis: {
      type: "category",
      data: channels.map((channel) => channel.label),
      splitArea: { show: false },
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { color: tc.muted, fontSize: 11 },
    },
    visualMap: {
      min: 0,
      max: Math.max(1, max),
      calculable: false,
      orient: "horizontal",
      left: "center",
      bottom: 0,
      itemWidth: 110,
      itemHeight: 8,
      textStyle: { color: tc.muted, fontSize: 11 },
      inRange: { color: ["#17202a", tc.blue, tc.accent, tc.green] },
    },
    series: [{
      type: "heatmap",
      data: matrix,
      emphasis: { itemStyle: { borderColor: tc.text, borderWidth: 1 } },
    }],
  }, true);
}

function renderTokenThroughput(tokensTime) {
  const data = tokensTime || [];
  const dates = data.map((row) => row.date);
  const tc = getThemeColors();
  const totals = data.map((row) =>
    Number(row.input_tokens || 0) + Number(row.output_tokens || 0) + Number(row.cache_read || 0) + Number(row.cache_create || 0)
  );
  const peak = totals.reduce((max, value) => Math.max(max, value), 0);
  setText("throughput-meta", `${t("total")}: ${fmt(totals.reduce((sum, value) => sum + value, 0))} · peak ${fmt(peak)}`);
  charts.tokens.setOption({
    ...baseOpt(),
    grid: { left: 62, right: 18, top: 44, bottom: 42 },
    dataZoom: [{ type: "inside", start: 0, end: 100 }],
    graphic: data.length === 0 ? emptyGraphic(t("noData")) : { type: "text", style: { text: "" } },
    tooltip: { ...baseOpt().tooltip, axisPointer: { type: "shadow" } },
    legend: {
      type: "scroll",
      top: 0,
      left: "center",
      textStyle: { color: tc.muted, fontSize: 11 },
      itemWidth: 10,
      itemHeight: 10,
      pageTextStyle: { color: tc.muted },
      pageIconColor: tc.muted,
    },
    xAxis: {
      type: "category",
      data: dates,
      axisLine: { lineStyle: { color: tc.grid } },
      axisTick: { show: false },
      axisLabel: { color: tc.muted, hideOverlap: true },
    },
    yAxis: {
      type: "value",
      splitLine: { lineStyle: { color: tc.grid } },
      axisLabel: { color: tc.muted, formatter: fmt },
    },
    series: [
      { name: t("input"), type: "bar", stack: "tokens", data: data.map((row) => row.input_tokens || 0), color: tc.blue, barMaxWidth: 38 },
      { name: t("output"), type: "bar", stack: "tokens", data: data.map((row) => row.output_tokens || 0), color: tc.green },
      { name: t("cacheRead"), type: "bar", stack: "tokens", data: data.map((row) => row.cache_read || 0), color: tc.purple },
      { name: t("cacheCreate"), type: "bar", stack: "tokens", data: data.map((row) => row.cache_create || 0), color: tc.amber },
    ],
  }, true);
}

function renderCostTrend(costTime, modelColorMap) {
  const rows = costTime || [];
  const dates = [...new Set(rows.map((row) => row.date))].sort();
  const totals = new Map();
  rows.forEach((row) => totals.set(row.model || t("unknownModel"), (totals.get(row.model || t("unknownModel")) || 0) + Number(row.value || 0)));
  const models = [...totals.keys()].sort((a, b) => totals.get(b) - totals.get(a));
  const valueMap = new Map();
  rows.forEach((row) => valueMap.set(`${row.date}\u0000${row.model || t("unknownModel")}`, Number(row.value || 0)));
  const tc = getThemeColors();
  charts.cost.setOption({
    ...baseOpt(),
    grid: { left: 62, right: 18, top: 48, bottom: 42 },
    dataZoom: [{ type: "inside", start: 0, end: 100 }],
    graphic: rows.length === 0 ? emptyGraphic(t("noData")) : { type: "text", style: { text: "" } },
    tooltip: {
      ...baseOpt().tooltip,
      axisPointer: { type: "shadow" },
      valueFormatter: fmtCost,
    },
    legend: {
      type: "scroll",
      top: 0,
      left: "center",
      textStyle: { color: tc.muted, fontSize: 11 },
      itemWidth: 10,
      itemHeight: 10,
      pageTextStyle: { color: tc.muted },
      pageIconColor: tc.muted,
    },
    xAxis: {
      type: "category",
      data: dates,
      axisLine: { lineStyle: { color: tc.grid } },
      axisTick: { show: false },
      axisLabel: { color: tc.muted, hideOverlap: true },
    },
    yAxis: {
      type: "value",
      splitLine: { lineStyle: { color: tc.grid } },
      axisLabel: { color: tc.muted, formatter: (value) => `$${value}` },
    },
    series: models.map((model) => ({
      name: model,
      type: "bar",
      stack: "cost",
      barMaxWidth: 38,
      color: modelColorMap.get(model) || COLORS[0],
      emphasis: { focus: "series" },
      data: dates.map((date) => Number((valueMap.get(`${date}\u0000${model}`) || 0).toFixed(4))),
    })),
  }, true);
}

function renderModelAllocation(costModel, modelColorMap) {
  const rows = (costModel || []).filter((row) => Number(row.cost || 0) > 0).slice(0, 12).reverse();
  const tc = getThemeColors();
  charts.models.setOption({
    ...baseOpt(),
    grid: { left: 130, right: 26, top: 12, bottom: 34 },
    graphic: rows.length === 0 ? emptyGraphic(t("noData")) : { type: "text", style: { text: "" } },
    tooltip: {
      ...baseOpt().tooltip,
      trigger: "axis",
      axisPointer: { type: "shadow" },
      valueFormatter: fmtCost,
    },
    xAxis: {
      type: "value",
      splitLine: { lineStyle: { color: tc.grid } },
      axisLabel: { color: tc.muted, formatter: (value) => `$${value}` },
    },
    yAxis: {
      type: "category",
      data: rows.map((row) => row.model || t("unknownModel")),
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: {
        color: tc.muted,
        width: 116,
        overflow: "truncate",
      },
    },
    series: [{
      type: "bar",
      data: rows.map((row) => ({
        value: Number(row.cost || 0),
        itemStyle: { color: modelColorMap.get(row.model || t("unknownModel")) || COLORS[0] },
      })),
      barMaxWidth: 18,
      label: {
        show: true,
        position: "right",
        color: tc.muted,
        formatter: (params) => fmtCost(params.value),
      },
    }],
  }, true);
}

async function refresh(options = {}) {
  if (isFetching) return;
  isFetching = true;
  $("btn-refresh").classList.add("loading");
  $("global-loader").classList.add("loading");
  updateRangeCaption();

  try {
    const [stats, costModel, costTime, tokensTime, sessions] = await Promise.all([
      api("stats"),
      api("cost-by-model", { skipModel: true }),
      api("cost-over-time"),
      api("tokens-over-time"),
      api("sessions"),
    ]);

    updateModelFilter(costModel || []);
    renderStats(stats || {});
    const modelColorMap = buildModelColorMap(costModel || []);
    renderActivityMatrix(tokensTime || []);
    renderTokenThroughput(tokensTime || []);
    renderCostTrend(costTime || [], modelColorMap);
    renderModelAllocation(costModel || [], modelColorMap);

    allSessions = sessions || [];
    renderSessionTable();
    if (!options.silent) showStatus(`${t("updated")} ${new Date().toLocaleTimeString()}`);
  } catch (err) {
    showStatus(`${t("refreshFailed")}: ${err.message}`, "error");
  } finally {
    isFetching = false;
    $("btn-refresh").classList.remove("loading");
    $("global-loader").classList.remove("loading");
  }
}

function parseTime(ts) {
  if (!ts) return null;
  const text = String(ts).replace(" +0000 UTC", "Z").replace(" UTC", "Z").replace(" ", "T");
  const d = new Date(text);
  return Number.isNaN(d.getTime()) ? null : d;
}

function relTime(ts) {
  const d = parseTime(ts);
  if (!d) return ts ? String(ts).replace("T", " ").slice(0, 16) : "-";
  const diff = Math.floor((Date.now() - d.getTime()) / 1000);
  if (diff < 0) return d.toLocaleString();
  if (diff < 60) return t("justNow");
  if (diff < 3600) return `${Math.floor(diff / 60)}${t("minAgo")}`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}${t("hourAgo")}`;
  if (diff < 604800) return `${Math.floor(diff / 86400)}${t("dayAgo")}`;
  return d.toLocaleDateString();
}

function fmtLocalTime(ts) {
  const d = parseTime(ts);
  return d ? d.toLocaleString() : (ts || "");
}

function sourceClass(source) {
  const key = String(source || "").toLowerCase();
  return KNOWN_SOURCES.has(key) ? `source-${key}` : "";
}

function createCell(text, className = "") {
  const td = document.createElement("td");
  if (className) td.className = className;
  td.textContent = text;
  return td;
}

function createSourceCell(source) {
  const td = document.createElement("td");
  const badge = document.createElement("span");
  badge.className = `badge ${sourceClass(source)}`.trim();
  badge.textContent = source || "-";
  td.appendChild(badge);
  return td;
}

function syncSortHeaders() {
  document.querySelectorAll(".sort-button").forEach((button) => {
    const mark = button.querySelector(".sort-mark");
    const active = button.dataset.sort === sessionSort.key;
    button.classList.toggle("active", active);
    mark.textContent = active ? (sessionSort.dir === "asc" ? "▲" : "▼") : "";
  });
}

function renderSessionTable() {
  const term = ($("filter-project").value || "").toLowerCase();
  const filtered = allSessions.filter((session) => {
    if (!term) return true;
    const haystack = `${session.project || ""} ${session.cwd || ""} ${session.git_branch || ""}`.toLowerCase();
    return haystack.includes(term);
  });

  const dir = sessionSort.dir === "asc" ? 1 : -1;
  const sorted = filtered.slice().sort((a, b) => {
    const key = sessionSort.key;
    const va = a[key] ?? "";
    const vb = b[key] ?? "";
    if (typeof va === "number" || typeof vb === "number") return ((Number(va) || 0) - (Number(vb) || 0)) * dir;
    return String(va).toLowerCase().localeCompare(String(vb).toLowerCase()) * dir;
  });

  const totalPages = Math.max(1, Math.ceil(sorted.length / PAGE_SIZE));
  if (sessionPage > totalPages) sessionPage = totalPages;
  const start = (sessionPage - 1) * PAGE_SIZE;
  const page = sorted.slice(start, start + PAGE_SIZE);
  const tbody = $("session-table");
  const fragment = document.createDocumentFragment();
  sessionKeyToID = new Map();

  if (page.length === 0) {
    const tr = document.createElement("tr");
    const td = createCell(t("noData"), "empty-state");
    td.colSpan = 8;
    tr.appendChild(td);
    fragment.appendChild(tr);
  } else {
    page.forEach((session, index) => {
      const sid = session.session_id || "";
      const key = `s${start + index}`;
      sessionKeyToID.set(key, sid);
      const isExpanded = expandedSessions.has(sid);
      const tr = document.createElement("tr");
      tr.className = `session-row${isExpanded ? " expanded" : ""}`;

      tr.appendChild(createSourceCell(session.source));
      const projectCell = createCell(session.project || session.cwd || "-", "project-cell");
      projectCell.title = session.cwd || session.project || "";
      tr.appendChild(projectCell);
      tr.appendChild(createCell(session.git_branch || "-", "muted-cell"));
      const timeCell = createCell(relTime(session.start_time), "mono");
      timeCell.title = fmtLocalTime(session.start_time);
      tr.appendChild(timeCell);
      tr.appendChild(createCell(fmt(session.prompts || 0), "num"));
      tr.appendChild(createCell(fmt(session.tokens || 0), "num"));
      tr.appendChild(createCell(fmtCost(session.total_cost || 0), "cost-cell"));

      const expandCell = document.createElement("td");
      const button = document.createElement("button");
      button.type = "button";
      button.className = `expand-btn${isExpanded ? " open" : ""}`;
      button.dataset.sessionKey = key;
      button.setAttribute("aria-expanded", isExpanded ? "true" : "false");
      button.setAttribute("aria-label", `${t("model")} ${sid}`);
      const icon = document.createElementNS("http://www.w3.org/2000/svg", "svg");
      icon.setAttribute("viewBox", "0 0 24 24");
      const path = document.createElementNS("http://www.w3.org/2000/svg", "path");
      path.setAttribute("d", "M9 5l7 7-7 7");
      icon.appendChild(path);
      button.appendChild(icon);
      expandCell.appendChild(button);
      tr.appendChild(expandCell);
      fragment.appendChild(tr);

      if (isExpanded) {
        const detail = buildDetailShell();
        fragment.appendChild(detail.row);
        fetchAndFillDetail(detail.content, sid);
      }
    });
  }

  tbody.replaceChildren(fragment);
  setText("ledger-meta", `${filtered.length} ${t("rows")}`);
  renderPagination(sorted.length, start, totalPages);
  syncSortHeaders();
}

function buildDetailShell() {
  const row = document.createElement("tr");
  row.className = "detail-row";
  const cell = document.createElement("td");
  cell.colSpan = 8;
  const content = document.createElement("div");
  content.className = "detail-content";
  const loading = document.createElement("div");
  loading.className = "loading-state";
  loading.textContent = t("loadingDetails");
  content.appendChild(loading);
  cell.appendChild(content);
  row.appendChild(cell);
  return { row, content };
}

function buildDetailTable(data) {
  const table = document.createElement("table");
  table.className = "detail-table";
  const thead = document.createElement("thead");
  const headerRow = document.createElement("tr");
  [t("model"), t("calls"), t("input"), t("output"), t("cacheRead"), t("cacheCreate"), t("cost")].forEach((label) => {
    const th = document.createElement("th");
    th.textContent = label;
    headerRow.appendChild(th);
  });
  thead.appendChild(headerRow);
  table.appendChild(thead);

  const tbody = document.createElement("tbody");
  data.forEach((row) => {
    const tr = document.createElement("tr");
    tr.appendChild(createCell(row.model || t("unknownModel"), "project-cell"));
    tr.appendChild(createCell(fmt(row.calls || 0), "num"));
    tr.appendChild(createCell(fmt(row.input_tokens || 0), "num"));
    tr.appendChild(createCell(fmt(row.output_tokens || 0), "num"));
    tr.appendChild(createCell(fmt(row.cache_read || 0), "num"));
    tr.appendChild(createCell(fmt(row.cache_create || 0), "num"));
    tr.appendChild(createCell(fmtCost(row.cost_usd || 0), "cost-cell"));
    tbody.appendChild(tr);
  });
  table.appendChild(tbody);
  return table;
}

async function fetchAndFillDetail(content, sid) {
  if (!sid) {
    content.replaceChildren(createMessage(t("noDetails"), "empty-state"));
    return;
  }
  try {
    const params = new URLSearchParams({ session_id: sid });
    const res = await fetch(`/api/session-detail?${params.toString()}`);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = await res.json();
    if (!Array.isArray(data) || data.length === 0) {
      content.replaceChildren(createMessage(t("noDetails"), "empty-state"));
      return;
    }
    content.replaceChildren(buildDetailTable(data));
  } catch (err) {
    content.replaceChildren(createMessage(`${t("detailFailed")} ${err.message}`, "empty-state"));
  }
}

function createMessage(text, className) {
  const div = document.createElement("div");
  div.className = className;
  div.textContent = text;
  return div;
}

function renderPagination(totalRows, start, totalPages) {
  const container = $("pagination");
  const fragment = document.createDocumentFragment();
  if (totalRows === 0) {
    container.replaceChildren();
    return;
  }

  const info = document.createElement("span");
  info.className = "page-info";
  if (totalPages <= 1) {
    info.textContent = `${totalRows} ${t("total")}`;
    fragment.appendChild(info);
    container.replaceChildren(fragment);
    return;
  }

  info.textContent = `${start + 1}-${Math.min(start + PAGE_SIZE, totalRows)} ${t("of")} ${totalRows}`;
  fragment.appendChild(info);

  const addButton = (label, page, active = false, disabled = false) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = `page-btn${active ? " active" : ""}`;
    button.textContent = label;
    button.dataset.page = String(page);
    button.disabled = disabled;
    fragment.appendChild(button);
  };

  addButton("‹", sessionPage - 1, false, sessionPage === 1);
  const pStart = Math.max(1, Math.min(sessionPage - 2, totalPages - 4));
  const pEnd = Math.min(totalPages, pStart + 4);
  for (let p = pStart; p <= pEnd; p += 1) addButton(String(p), p, p === sessionPage);
  addButton("›", sessionPage + 1, false, sessionPage === totalPages);
  container.replaceChildren(fragment);
}

function updateModelFilter(costModel) {
  const models = [...new Set((costModel || []).map((row) => row.model).filter(Boolean))];
  if (state.model && !models.includes(state.model)) persist("model", "");
  fillSelect($("filter-model"), [
    { value: "", label: t("allModels") },
    ...models.map((model) => ({ value: model, label: model })),
  ], state.model);
}

function buildControls() {
  document.documentElement.lang = state.lang;
  document.title = "agent-usage";
  document.querySelectorAll("[data-i18n]").forEach((el) => {
    el.textContent = t(el.dataset.i18n);
  });

  fillSelect($("sel-theme"), [
    { value: "system", label: t("system") },
    { value: "light", label: t("light") },
    { value: "dark", label: t("dark") },
  ], state.theme);
  fillSelect($("sel-lang"), [
    { value: "en", label: "EN" },
    { value: "zh", label: "ZH" },
  ], state.lang);
  fillSelect($("sel-granularity"), GRANULARITIES.map((value) => ({ value, label: t(`gran_${value}`) })), state.granularity);
  fillSelect($("sel-refresh-interval"), REFRESH_INTERVALS.map((value) => ({
    value: String(value),
    label: value >= 60 ? `${value / 60} ${t("unitMin")}` : `${value} ${t("unitSec")}`,
  })), String(state.refreshInterval));
  fillSelect($("filter-source"), SOURCES.map(([value, labelKey]) => ({ value, label: t(labelKey) })), state.source);
  fillSelect($("filter-model"), [
    { value: "", label: t("allModels") },
    ...(state.model ? [{ value: state.model, label: state.model }] : []),
  ], state.model);

  const presetFragment = document.createDocumentFragment();
  PRESETS.forEach((preset) => {
    const button = document.createElement("button");
    button.type = "button";
    button.dataset.preset = preset;
    button.className = state.preset === preset ? "active" : "";
    button.textContent = t(preset);
    presetFragment.appendChild(button);
  });
  $("preset-bar").replaceChildren(presetFragment);
  $("filter-project").placeholder = t("filterProject");
  updateRangeCaption();
  applyAutoRefresh();
  syncSortHeaders();
}

function applyAutoRefresh() {
  if (autoTimer) {
    clearInterval(autoTimer);
    autoTimer = null;
  }
  $("auto-status").textContent = state.autoRefresh ? t("autoOn") : t("autoOff");
  $("btn-auto-refresh").classList.toggle("active", state.autoRefresh);
  if (state.autoRefresh) autoTimer = setInterval(() => refresh({ silent: true }), state.refreshInterval * 1000);
}

$("sel-theme").addEventListener("change", (e) => {
  persist("theme", e.target.value);
  applyTheme();
  refresh();
});

$("sel-lang").addEventListener("change", (e) => {
  persist("lang", e.target.value);
  buildControls();
  refresh();
});

$("sel-granularity").addEventListener("change", (e) => {
  persist("granularity", e.target.value);
  updateRangeCaption();
  refresh();
});

$("sel-refresh-interval").addEventListener("change", (e) => {
  persist("refreshInterval", Number(e.target.value));
  applyAutoRefresh();
});

$("filter-source").addEventListener("change", (e) => {
  persist("source", e.target.value);
  persist("model", "");
  sessionPage = 1;
  refresh();
});

$("filter-model").addEventListener("change", (e) => {
  persist("model", e.target.value);
  sessionPage = 1;
  refresh();
});

$("filter-project").addEventListener("input", () => {
  sessionPage = 1;
  renderSessionTable();
});

$("from").addEventListener("change", (e) => {
  persist("customFrom", e.target.value);
  updateRangeCaption();
  refresh();
});

$("to").addEventListener("change", (e) => {
  persist("customTo", e.target.value);
  updateRangeCaption();
  refresh();
});

$("preset-bar").addEventListener("click", (e) => {
  const button = e.target.closest("button[data-preset]");
  if (!button) return;
  persist("preset", button.dataset.preset);
  buildControls();
  refresh();
});

$("btn-refresh").addEventListener("click", () => {
  refresh();
  applyAutoRefresh();
});

$("btn-auto-refresh").addEventListener("click", () => {
  persist("autoRefresh", !state.autoRefresh);
  applyAutoRefresh();
});

$("session-table").addEventListener("click", (e) => {
  const button = e.target.closest(".expand-btn");
  if (!button) return;
  const sid = sessionKeyToID.get(button.dataset.sessionKey) || "";
  const row = button.closest(".session-row");
  const next = row ? row.nextElementSibling : null;
  if (expandedSessions.has(sid)) {
    expandedSessions.delete(sid);
    button.classList.remove("open");
    button.setAttribute("aria-expanded", "false");
    if (row) row.classList.remove("expanded");
    if (next && next.classList.contains("detail-row")) next.remove();
    return;
  }

  expandedSessions.add(sid);
  button.classList.add("open");
  button.setAttribute("aria-expanded", "true");
  if (row) {
    row.classList.add("expanded");
    const detail = buildDetailShell();
    row.after(detail.row);
    fetchAndFillDetail(detail.content, sid);
  }
});

$("pagination").addEventListener("click", (e) => {
  const button = e.target.closest(".page-btn:not(:disabled)");
  if (!button || button.classList.contains("active")) return;
  sessionPage = Number(button.dataset.page);
  renderSessionTable();
});

document.querySelectorAll(".sort-button").forEach((button) => {
  button.addEventListener("click", () => {
    const key = button.dataset.sort;
    if (sessionSort.key === key) {
      sessionSort.dir = sessionSort.dir === "asc" ? "desc" : "asc";
    } else {
      sessionSort.key = key;
      sessionSort.dir = ["start_time", "total_cost", "tokens", "prompts"].includes(key) ? "desc" : "asc";
    }
    renderSessionTable();
  });
});

window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => {
  if (state.theme === "system") {
    applyTheme();
    refresh();
  }
});

applyTheme();
initCharts();
buildControls();
refresh({ silent: true });
