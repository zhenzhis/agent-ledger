const $ = (id) => document.getElementById(id);
const PRODUCT_NAME = "Agent Ledger";

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

const fmtSignedCost = (value) => {
  const n = Number(value || 0);
  const sign = n > 0 ? "+" : "";
  const minus = n < 0 ? "-" : "";
  return `${sign}${minus}${fmtCost(Math.abs(n))}`;
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
    budget: "Budget",
    health: "Health",
    battery: "Battery",
    pricing: "Pricing",
    quality: "Quality",
    modelCalls: "Model Calls",
    window: "Window",
    filters: "Filters",
    operations: "Operations",
    preferences: "Preferences",
    advanced: "Advanced",
    appTagline: "Local AI agent usage ledger",
    activityMatrix: "Activity Matrix",
    tokenThroughput: "Token Throughput",
    costOverTime: "Cost Trend",
    modelAllocation: "Model Allocation",
    budgetStatus: "Budget Status",
    ingestionHealth: "Ingestion Health",
    agentBattery: "Agent Battery",
    pricingHealth: "Pricing Health",
    costIntelligence: "Cost Intelligence",
    cacheDoctor: "Cache Doctor",
    dataQuality: "Data Quality",
    watchdog: "Watchdog",
    auditLog: "Audit Log",
    fleetAttribution: "Fleet Attribution",
    reconciliation: "Reconciliation",
    teamShowback: "Team Showback",
    team: "Team",
    workloadLedger: "Workload Ledger",
    timeline: "Timeline",
    goal: "Goal",
    status: "Status",
    observerMode: "Observer Mode",
    controlPlaneMode: "Control Plane",
    readOnlyActionDisabled: "Read-only observer mode: write operations are disabled",
    outcome: "Outcome",
    runs: "runs",
    toolCalls: "tool calls",
    contextRefs: "context refs",
    refType: "Type",
    refHash: "Hash",
    privacy: "Privacy",
    tool: "Tool",
    duration: "Duration",
    confidence: "confidence",
    sessionLedger: "Session Ledger",
    filterProject: "Project / workspace",
    ledgerSearch: "Search ledger by project, path, or branch...",
    scanNow: "Scan",
    scanSource: "Scan Source",
    pricingSync: "Pricing",
    doctor: "Doctor",
    recalcCosts: "Rebuild Costs",
    repairProjections: "Repair Ledger",
    resetScan: "Clean Rescan",
    exportCsv: "Export CSV",
    reportMd: "Markdown Report",
    privacyOn: "Privacy On",
    privacyOff: "Privacy",
    scanStarted: "Scan started",
    scanDone: "Scan completed",
    recalcDone: "Cost rebuild completed",
    repairStarted: "Repairing ledger projections",
    repairDone: "Ledger projection repair completed",
    pricingDone: "Pricing sync completed",
    resetConfirm: "Reset scan state and usage for current source, then rescan?",
    resetNeedsSource: "Choose one source before reset",
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
    effectiveRules: "Effective Rules",
    overrideRules: "overrides",
    officialRules: "official",
    fallbackRules: "fallback",
    sourcesLabel: "sources",
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
    partialRefreshFailed: "Some panels failed",
    actionFailed: "Action failed",
    disabled: "disabled",
    missingPath: "missing path",
    unreadablePath: "unreadable path",
    lastError: "last error",
    noBudgets: "No budget rules configured",
    noHealth: "No scan health yet",
    warning: "warning",
    critical: "critical",
    ok: "ok",
    unknownModel: "Unknown model",
    rows: "rows",
    records: "records",
    issuesLabel: "issues",
    inserted: "inserted",
    updatedRows: "updated",
    unitMin: "min",
    unitSec: "sec",
    noIssues: "No issues detected",
    noAuditEvents: "No audit events",
    noFleet: "No fleet runs",
    noReconciliation: "No provider statements imported",
    noChargeback: "No showback rows",
    providerBill: "provider bill",
    localLedger: "local ledger",
    ledgerProjection: "Ledger Projection",
    policyAudit: "Policy Audit",
    checked: "checked",
    matches: "matches",
    blocks: "blocks",
    approvals: "approvals",
    warnings: "warnings",
    projectionCalls: "model calls",
    projectionIssues: "projection issues",
    unpriced: "unpriced",
    stale: "stale",
    runLiveness: "Run Liveness",
    staleRuns: "stale runs",
    activeRuns: "active runs",
    heartbeat: "heartbeat",
    lastActive: "last active",
    progress: "progress",
    localEstimate: "local estimate",
    terminalState: "Terminal State",
    phase: "Phase",
    readiness: "Readiness",
    nextAction: "Next Action",
    reasons: "Reasons",
    risks: "Risks",
    noRisks: "No risks",
    terminal: "terminal",
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
    budget: "预算",
    health: "健康",
    battery: "电量",
    pricing: "价格",
    quality: "质量",
    modelCalls: "模型调用",
    window: "时间窗口",
    filters: "过滤条件",
    operations: "操作",
    preferences: "偏好",
    advanced: "高级",
    appTagline: "本地 AI Agent 用量账本",
    activityMatrix: "活动矩阵",
    tokenThroughput: "Token 吞吐",
    costOverTime: "费用趋势",
    modelAllocation: "模型分布",
    budgetStatus: "预算状态",
    ingestionHealth: "采集健康",
    agentBattery: "Agent 电量",
    pricingHealth: "价格健康",
    costIntelligence: "成本解释",
    cacheDoctor: "Cache Doctor",
    dataQuality: "数据质量",
    watchdog: "Watchdog",
    auditLog: "审计日志",
    fleetAttribution: "Fleet Attribution",
    reconciliation: "对账",
    teamShowback: "团队 Showback",
    team: "团队",
    workloadLedger: "工作负载账本",
    timeline: "时间线",
    goal: "目标",
    status: "状态",
    observerMode: "观测模式",
    controlPlaneMode: "控制面",
    readOnlyActionDisabled: "只读观测模式：写操作已禁用",
    outcome: "结果",
    runs: "次运行",
    toolCalls: "工具调用",
    contextRefs: "上下文引用",
    refType: "类型",
    refHash: "Hash",
    privacy: "隐私",
    tool: "工具",
    duration: "耗时",
    confidence: "可信度",
    sessionLedger: "会话账本",
    filterProject: "项目 / 工作区",
    ledgerSearch: "按项目、路径或分支搜索账本...",
    scanNow: "扫描",
    scanSource: "扫描来源",
    pricingSync: "同步价格",
    doctor: "诊断",
    recalcCosts: "重建费用",
    repairProjections: "修复账本",
    resetScan: "清理重扫",
    exportCsv: "导出 CSV",
    reportMd: "Markdown 报告",
    privacyOn: "隐私开启",
    privacyOff: "隐私",
    scanStarted: "开始扫描",
    scanDone: "扫描完成",
    recalcDone: "费用重建完成",
    repairStarted: "正在修复账本投影",
    repairDone: "账本投影修复完成",
    pricingDone: "价格同步完成",
    resetConfirm: "清理当前来源的扫描状态和用量后重新扫描？",
    resetNeedsSource: "清理重扫前请选择单个来源",
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
    effectiveRules: "有效规则",
    overrideRules: "override",
    officialRules: "官方",
    fallbackRules: "fallback",
    sourcesLabel: "来源",
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
    partialRefreshFailed: "部分面板刷新失败",
    actionFailed: "操作失败",
    disabled: "已禁用",
    missingPath: "路径不存在",
    unreadablePath: "路径不可读",
    lastError: "最近错误",
    noBudgets: "未配置预算规则",
    noHealth: "暂无采集健康状态",
    warning: "警告",
    critical: "严重",
    ok: "正常",
    unknownModel: "未知模型",
    rows: "行",
    records: "条记录",
    issuesLabel: "问题",
    inserted: "已补回",
    updatedRows: "已更新",
    unitMin: "分钟",
    unitSec: "秒",
    noIssues: "未发现问题",
    noAuditEvents: "暂无审计事件",
    noFleet: "暂无 Fleet 归因数据",
    noReconciliation: "尚未导入 provider 账单",
    noChargeback: "暂无团队归因数据",
    providerBill: "provider 账单",
    localLedger: "本地账本",
    ledgerProjection: "账本投影",
    policyAudit: "策略审计",
    checked: "已检查",
    matches: "匹配",
    blocks: "阻断",
    approvals: "审批",
    warnings: "警告",
    projectionCalls: "模型调用",
    projectionIssues: "投影问题",
    unpriced: "未计价",
    stale: "过期",
    runLiveness: "运行存活",
    staleRuns: "失联运行",
    activeRuns: "活跃运行",
    heartbeat: "心跳",
    lastActive: "最近活动",
    progress: "进度",
    localEstimate: "本地估算",
    terminalState: "终态快照",
    phase: "阶段",
    readiness: "就绪度",
    nextAction: "下一步",
    reasons: "依据",
    risks: "风险",
    noRisks: "无风险",
    terminal: "终态",
  },
};

const PRESETS = ["today", "thisWeek", "thisMonth", "thisYear", "last3d", "last7d", "last30d", "custom"];
const GRANULARITIES = ["1m", "30m", "1h", "6h", "12h", "1d", "1w", "1M"];
const REFRESH_INTERVALS = [30, 60, 300, 1800, 3600];
const COLORS = ["#f5f5f5", "#d4d4d4", "#a3a3a3", "#737373", "#525252", "#bdbdbd", "#8a8a8a", "#e5e5e5", "#6b6b6b", "#c7c7c7"];
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
const PAGE_SIZE = 50;

const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
const urlOptions = new URLSearchParams(window.location.search);
const privacyParam = urlOptions.get("privacy");

function flagEnabled(value) {
  return value === "1" || value === "true" || value === "yes";
}

let state = {
  lang: localStorage.getItem("au-lang") || (navigator.language.toLowerCase().startsWith("zh") ? "zh" : "en"),
  theme: localStorage.getItem("au-theme") || "dark",
  preset: localStorage.getItem("au-preset") || "today",
  granularity: localStorage.getItem("au-granularity") || "1h",
  autoRefresh: localStorage.getItem("au-autoRefresh") !== "false",
  refreshInterval: Number(localStorage.getItem("au-refreshInterval") || 60),
  customFrom: localStorage.getItem("au-customFrom") || "",
  customTo: localStorage.getItem("au-customTo") || "",
  source: localStorage.getItem("au-source") || "",
  model: localStorage.getItem("au-model") || "",
  project: localStorage.getItem("au-project") || "",
  ledgerQuery: localStorage.getItem("au-ledgerQuery") || "",
  privacy: privacyParam === null ? localStorage.getItem("au-privacy") === "true" : flagEnabled(privacyParam),
  runtime: null,
};

let charts = {};
let autoTimer = null;
let statusTimer = null;
let allWorkloads = [];
let workloadTotal = 0;
let expandedWorkloads = new Set();
let allSessions = [];
let sessionTotal = 0;
let sessionSort = { key: "last_activity", dir: "desc" };
let sessionPage = 1;
let expandedSessions = new Set();
let sessionKeyToID = new Map();
let isFetching = false;
let projectFilterTimer = null;

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
    accent: cs.getPropertyValue("--accent").trim() || "#f5f5f5",
    green: cs.getPropertyValue("--green").trim() || "#d4d4d4",
    amber: cs.getPropertyValue("--amber").trim() || "#a3a3a3",
    blue: cs.getPropertyValue("--blue").trim() || "#f5f5f5",
    purple: cs.getPropertyValue("--purple").trim() || "#737373",
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

function dateRangeDays() {
  const range = getTimeRange();
  const from = new Date(`${range.from}T00:00:00`);
  const to = new Date(`${range.to}T00:00:00`);
  if (Number.isNaN(from.getTime()) || Number.isNaN(to.getTime())) return 1;
  return Math.max(1, Math.floor((to.getTime() - from.getTime()) / 86400000) + 1);
}

function effectiveGranularity() {
  const days = dateRangeDays();
  const g = state.granularity;
  if (days > 90 && ["1m", "30m", "1h", "6h", "12h"].includes(g)) return "1d";
  if (days > 14 && ["1m", "30m"].includes(g)) return "1h";
  if (days > 2 && g === "1m") return "30m";
  return g;
}

function updateRangeCaption() {
  const range = getTimeRange();
  const granularity = effectiveGranularity();
  const granText = granularity === state.granularity
    ? t(`gran_${granularity}`)
    : `${t(`gran_${state.granularity}`)} -> ${t(`gran_${granularity}`)}`;
  setText("range-caption", `${range.from} ${t("to")} ${range.to} · ${granText}`);
  $("custom-range-wrap").classList.toggle("is-hidden", state.preset !== "custom");
  $("from").value = state.customFrom || range.from;
  $("to").value = state.customTo || range.to;
}

function applyRuntimeStatus(runtime) {
  state.runtime = runtime || null;
  const readOnly = Boolean(runtime && runtime.read_only);
  const caption = $("runtime-caption");
  if (caption) {
    caption.textContent = readOnly ? t("observerMode") : t("controlPlaneMode");
    caption.title = (runtime && runtime.message) || "";
    caption.classList.toggle("is-hidden", !runtime);
    caption.classList.toggle("is-observer", readOnly);
  }
  const disabledTitle = readOnly ? t("readOnlyActionDisabled") : "";
  ["btn-scan", "btn-pricing-sync", "btn-recalc", "btn-repair-projections", "btn-reset-scan"].forEach((id) => {
    const button = $(id);
    if (!button) return;
    button.disabled = readOnly;
    button.title = disabledTitle;
    button.setAttribute("aria-disabled", readOnly ? "true" : "false");
  });
}

function buildParams(opts = {}) {
  const range = getTimeRange();
  const params = new URLSearchParams({
    from: range.from,
    to: range.to,
    tz_offset: String(new Date().getTimezoneOffset()),
  });
  if (state.granularity) params.set("granularity", effectiveGranularity());
  if (state.source) params.set("source", state.source);
  if (state.model && !opts.skipModel) params.set("model", state.model);
  if (state.project) params.set("project", state.project);
  if (state.privacy) params.set("privacy", "1");
  Object.entries(opts.extra || {}).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== "") params.set(key, String(value));
  });
  return params;
}

async function api(path, opts = {}) {
  const params = buildParams(opts);
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

async function postApi(path, opts = {}) {
  const params = buildParams(opts);
  const res = await fetch(`/api/${path}?${params.toString()}`, { method: "POST" });
  let body = null;
  try {
    body = await res.json();
  } catch (err) {
    if (res.ok) return {};
  }
  if (!res.ok) throw new Error((body && body.error) || `${path}: HTTP ${res.status}`);
  return body || {};
}

function downloadApi(path, opts = {}) {
  const params = buildParams(opts);
  window.location.href = `/api/${path}?${params.toString()}`;
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

function renderBudgets(payload) {
  const rules = (payload && payload.rules) || [];
  const list = $("budget-list");
  const fragment = document.createDocumentFragment();
  let worst = "ok";
  if (rules.length === 0) {
    fragment.appendChild(createMessage(t("noBudgets"), "ops-empty"));
    setText("s-budget", t("ok").toUpperCase());
    setText("s-budget-sub", t("noBudgets"));
    setText("budget-meta", payload && payload.enabled ? "0" : t("disabled"));
  } else {
    rules.forEach((rule) => {
      if (rule.severity === "critical") worst = "critical";
      else if (rule.severity === "warning" && worst !== "critical") worst = "warning";
      const row = document.createElement("div");
      row.className = `ops-row severity-${rule.severity || "ok"}`;
      const main = document.createElement("div");
      main.className = "ops-main";
      const title = document.createElement("strong");
      title.textContent = rule.name || "-";
      const sub = document.createElement("span");
      sub.textContent = `${rule.scope || "global"}${rule.match ? `:${rule.match}` : ""} · ${rule.metric || "cost_usd"}`;
      main.append(title, sub);
      const value = document.createElement("div");
      value.className = "ops-value";
      const pct = Number(rule.ratio || 0) * 100;
      value.textContent = `${pct.toFixed(0)}%`;
      row.append(main, value);
      fragment.appendChild(row);
    });
    const maxRule = rules.reduce((best, row) => Number(row.ratio || 0) > Number((best && best.ratio) || 0) ? row : best, null);
    setText("s-budget", t(worst).toUpperCase());
    setText("s-budget-sub", maxRule ? `${maxRule.name}: ${(Number(maxRule.ratio || 0) * 100).toFixed(0)}%` : "-");
    setText("budget-meta", `${rules.length} rules`);
  }
  list.replaceChildren(fragment);
}

function renderHealth(rows) {
  const health = rows || [];
  const list = $("health-list");
  const fragment = document.createDocumentFragment();
  let problemCount = 0;
  if (health.length === 0) {
    fragment.appendChild(createMessage(t("noHealth"), "ops-empty"));
    setText("s-health", t("ok").toUpperCase());
    setText("s-health-sub", t("noHealth"));
    setText("health-meta", "0");
  } else {
    health.forEach((row) => {
      const pathIssues = (row.path_status || []).filter((p) => !p.exists || !p.readable);
      const hasError = Boolean(row.last_error && row.enabled);
      const disabled = !row.enabled;
      if ((pathIssues.length > 0 || hasError) && !disabled) problemCount += 1;
      const status = disabled ? "disabled" : hasError || pathIssues.length ? "warning" : "ok";
      const item = document.createElement("div");
      item.className = `ops-row severity-${status}`;
      const main = document.createElement("div");
      main.className = "ops-main";
      const title = document.createElement("strong");
      title.textContent = row.source || "-";
      const detail = document.createElement("span");
      if (disabled) detail.textContent = t("disabled");
      else if (hasError) detail.textContent = `${t("lastError")}: ${row.last_error}`;
      else if (pathIssues.length) detail.textContent = pathIssues.map((p) => p.exists ? t("unreadablePath") : t("missingPath")).join(", ");
      else {
        const lastScan = row.last_scan_at ? relTime(row.last_scan_at) : "-";
        detail.textContent = `${fmt(row.records_inserted || 0)} ${t("records")} · ${fmt(row.prompts_inserted || 0)} ${t("prompts")} · ${lastScan} · ${fmt(row.duration_ms || 0)} ms`;
      }
      main.append(title, detail);
      const value = document.createElement("div");
      value.className = "ops-value";
      value.textContent = disabled ? "-" : fmt(row.files_seen || 0);
      item.append(main, value);
      fragment.appendChild(item);
    });
    setText("s-health", problemCount ? String(problemCount) : t("ok").toUpperCase());
    setText("s-health-sub", `${health.length} ${t("sourcesLabel")}`);
    setText("health-meta", `${problemCount} ${t("issuesLabel")}`);
  }
  list.replaceChildren(fragment);
}

function addOpsRow(fragment, title, detail, value = "", severity = "ok") {
  const row = document.createElement("div");
  row.className = `ops-row severity-${severity || "ok"}`;
  const main = document.createElement("div");
  main.className = "ops-main";
  const strong = document.createElement("strong");
  strong.textContent = title || "-";
  const sub = document.createElement("span");
  sub.textContent = detail || "";
  main.append(strong, sub);
  const val = document.createElement("div");
  val.className = "ops-value";
  val.textContent = value;
  row.append(main, val);
  fragment.appendChild(row);
}

function renderQuota(payload) {
  const list = $("quota-list");
  if (!list) return;
  const fragment = document.createDocumentFragment();
  const windows = (payload && payload.windows) || [];
  if (windows.length === 0) {
    fragment.appendChild(createMessage(t("localEstimate"), "ops-empty"));
    setText("quota-meta", "-");
    setText("s-battery", "-");
    setText("s-battery-sub", t("localEstimate"));
  } else {
    windows.forEach((row) => {
      const ratio = row.cost_limit > 0 ? Number(row.cost_usd || 0) / Number(row.cost_limit || 1) : 0;
      const severity = ratio >= 1 ? "critical" : ratio >= 0.8 ? "warning" : "ok";
      const eta = Number(row.time_to_limit_hours || -1);
      const etaText = eta >= 0 ? ` · ETA ${eta.toFixed(1)}h` : "";
      const resetText = row.reset_at ? ` · reset ${relTime(row.reset_at)}` : "";
      addOpsRow(fragment, row.name, `${fmtCost(row.cost_usd)} · ${fmt(row.tokens)} tokens · ${fmtCost(row.burn_rate_per_hour)}/h${etaText}${resetText}`, row.cost_limit > 0 ? `${(ratio * 100).toFixed(0)}%` : "-", severity);
    });
    const month = windows.find((row) => row.name === "month") || windows[0];
    const ratio = month.cost_limit > 0 ? Number(month.cost_usd || 0) / Number(month.cost_limit || 1) : 0;
    setText("s-battery", month.cost_limit > 0 ? `${(Math.max(0, 1 - ratio) * 100).toFixed(0)}%` : "∞");
    setText("s-battery-sub", `${payload.plan || "custom"} · ${t("localEstimate")}`);
    setText("quota-meta", `${windows.length} windows`);
  }
  list.replaceChildren(fragment);
}

function renderPricing(payload) {
  const list = $("pricing-list");
  if (!list) return;
  const fragment = document.createDocumentFragment();
  const sources = (payload && payload.sources) || [];
  const rules = (payload && payload.rules) || {};
  const stale = sources.filter((s) => s.stale).length;
  const sourceErrors = sources.filter((s) => s.status === "error").length;
  const unpriced = ((payload && payload.unpriced_models) || []).reduce((sum, row) => sum + Number(row.records || 0), 0);
  const totalRules = Number(rules.total_rules || 0);
  if (totalRules > 0) {
    const detail = `${fmt(rules.override_rules || 0)} ${t("overrideRules")} · ${fmt(rules.official_rules || 0)} ${t("officialRules")} · ${fmt(rules.fallback_rules || 0)} ${t("fallbackRules")}`;
    addOpsRow(fragment, t("effectiveRules"), detail, fmt(totalRules), sourceErrors || unpriced ? "warning" : "ok");
  }
  if (sources.length === 0 && totalRules === 0) {
    fragment.appendChild(createMessage(t("noData"), "ops-empty"));
  } else {
    sources.forEach((src) => {
      addOpsRow(fragment, src.name, `${src.kind || "-"} · ${src.status || "-"} · ${src.model_count || 0} models`, src.stale ? t("stale") : "ok", src.stale || src.status === "error" ? "warning" : "ok");
    });
  }
  setText("s-pricing", sourceErrors ? `${sourceErrors} ${t("warning")}` : stale ? `${stale} ${t("stale")}` : "OK");
  setText("s-pricing-sub", unpriced ? `${fmt(unpriced)} ${t("unpriced")}` : payload ? payload.mode : "-");
  setText("pricing-meta", `${sources.length} ${t("sourcesLabel")} · ${fmt(totalRules)} ${t("effectiveRules")}`);
  list.replaceChildren(fragment);
}

function renderQuality(payload, policyAudit) {
  const list = $("quality-list");
  if (!list) return;
  const fragment = document.createDocumentFragment();
  const rows = (payload && payload.source_quality) || [];
  const projection = payload && payload.projection;
  const projectionIssues = projection
    ? Number(projection.missing_usage_projection || 0) + Number(projection.cost_mismatch_records || 0) + Number(projection.duplicate_session_owners || 0)
    : 0;
  const hasProjectionSignal = Boolean(projection && (Number(projection.model_calls || 0) > 0 || projectionIssues > 0));
  const auditChecked = Number((policyAudit && policyAudit.checked) || 0);
  const auditMatches = Number((policyAudit && policyAudit.matches) || 0);
  const auditBlocks = Number((policyAudit && policyAudit.blocks) || 0);
  const auditApprovals = Number((policyAudit && policyAudit.approvals) || 0);
  const auditWarnings = Number((policyAudit && policyAudit.warnings) || 0);
  const hasAuditSignal = Boolean(policyAudit && (auditChecked > 0 || auditMatches > 0));
  if (rows.length === 0 && !hasProjectionSignal && !hasAuditSignal) {
    fragment.appendChild(createMessage(t("noData"), "ops-empty"));
    setText("s-quality", "-");
    setText("s-quality-sub", "-");
  } else {
    let min = 1;
    if (hasProjectionSignal) {
      const projectionConfidence = Number(projection.confidence || 0);
      min = Math.min(min, projectionConfidence);
      const severity = projectionIssues > 0 || projectionConfidence < 0.8 ? "warning" : "ok";
      const detail = `${projection.message || "-"} · ${fmt(projection.model_calls || 0)} ${t("projectionCalls")}`;
      addOpsRow(fragment, t("ledgerProjection"), detail, `${(projectionConfidence * 100).toFixed(0)}%`, severity);
    }
    if (hasAuditSignal) {
      const severity = auditBlocks > 0 ? "critical" : auditApprovals > 0 || auditWarnings > 0 ? "warning" : "ok";
      const auditConfidence = auditBlocks > 0 ? 0.5 : auditApprovals > 0 || auditWarnings > 0 ? 0.75 : 1;
      min = Math.min(min, auditConfidence);
      const detail = `${fmt(auditChecked)} ${t("checked")} · ${fmt(auditBlocks)} ${t("blocks")} · ${fmt(auditApprovals)} ${t("approvals")} · ${fmt(auditWarnings)} ${t("warnings")}`;
      addOpsRow(fragment, t("policyAudit"), detail, auditMatches ? `${fmt(auditMatches)} ${t("matches")}` : "OK", severity);
    }
    rows.forEach((row) => {
      min = Math.min(min, Number(row.confidence || 0));
      const sev = row.confidence < 0.7 ? "warning" : "ok";
      addOpsRow(fragment, row.source, row.message || `${row.records || 0} ${t("records")}`, `${(Number(row.confidence || 0) * 100).toFixed(0)}%`, sev);
    });
    setText("s-quality", `${(min * 100).toFixed(0)}%`);
    const qualityIssues = projectionIssues + auditMatches;
    setText("s-quality-sub", qualityIssues ? `${rows.length} ${t("sourcesLabel")} · ${qualityIssues} ${t("issuesLabel")}` : `${rows.length} ${t("sourcesLabel")}`);
  }
  const projectionMeta = hasProjectionSignal ? ` · ${fmt(projection.model_calls || 0)} ${t("projectionCalls")}` : "";
  const policyMeta = hasAuditSignal ? ` · ${fmt(auditMatches)} ${t("matches")}` : "";
  setText("quality-meta", `${((payload && payload.unpriced_models) || []).length} ${t("unpriced")}${projectionMeta}${policyMeta}`);
  list.replaceChildren(fragment);
}

function renderModelCalls(rows) {
  const list = $("calls-list");
  if (!list) return;
  const fragment = document.createDocumentFragment();
  const total = (rows || []).reduce((sum, row) => sum + Number(row.calls || 0), 0);
  (rows || []).slice(0, 8).forEach((row) => {
    addOpsRow(fragment, row.model || t("unknownModel"), `${row.source} · ${row.project || "-"} · ${fmt(row.avg_tokens_per_call)} tokens/call`, fmt(row.calls || 0), row.unpriced_calls ? "warning" : "ok");
  });
  if (!rows || rows.length === 0) fragment.appendChild(createMessage(t("noData"), "ops-empty"));
  setText("s-model-calls", fmt(total));
  setText("s-model-calls-sub", `${(rows || []).length} groups`);
  setText("calls-meta", `${fmt(total)} ${t("calls")}`);
  list.replaceChildren(fragment);
}

function renderCostIntelligence(rows) {
  const list = $("cost-intel-list");
  if (!list) return;
  const fragment = document.createDocumentFragment();
  (rows || []).slice(0, 8).forEach((row) => {
    const reason = (row.reasons || [])[0] || "-";
    addOpsRow(fragment, `${row.source} · ${row.project || "-"}`, `${reason} · score ${(Number(row.quality_score || 0) * 100).toFixed(0)}%`, fmtCost(row.cost_usd || 0), row.quality_score < 0.7 ? "warning" : "ok");
  });
  if (!rows || rows.length === 0) fragment.appendChild(createMessage(t("noData"), "ops-empty"));
  setText("cost-intel-meta", `${(rows || []).length} sessions`);
  list.replaceChildren(fragment);
}

function renderCacheDoctor(rows) {
  const list = $("cache-list");
  if (!list) return;
  const fragment = document.createDocumentFragment();
  (rows || []).slice(0, 8).forEach((row) => {
    const hit = Number(row.cache_hit_rate || 0);
    addOpsRow(fragment, row.model || t("unknownModel"), `${row.source} · ${row.message || ""}`, `${(hit * 100).toFixed(0)}%`, hit < 0.25 ? "warning" : "ok");
  });
  if (!rows || rows.length === 0) fragment.appendChild(createMessage(t("noData"), "ops-empty"));
  setText("cache-meta", `${(rows || []).length} groups`);
  list.replaceChildren(fragment);
}

function runLivenessSeverity(row) {
  const status = String(row.status || "").toLowerCase();
  if (row.stale) return "critical";
  if (status === "blocked" || status === "stalled" || status === "waiting_approval") return "warning";
  return "ok";
}

function renderWatchdog(rows, liveness) {
  const list = $("watchdog-list");
  if (!list) return;
  const fragment = document.createDocumentFragment();
  const events = rows || [];
  const runs = ((liveness && liveness.rows) || []).slice().sort((a, b) => {
    if (a.stale !== b.stale) return a.stale ? -1 : 1;
    return Number(b.age_seconds || 0) - Number(a.age_seconds || 0);
  });
  runs.slice(0, 4).forEach((row) => {
    const label = row.stale ? t("staleRuns") : t("activeRuns");
    const title = `${label} · ${row.source || "-"} · ${row.agent_name || "-"}`;
    const last = row.last_activity ? relTime(row.last_activity) : "-";
    const detail = `${row.goal || row.run_id || "-"} · ${row.phase || row.status || "-"} · ${t("lastActive")} ${last}`;
    const progress = Number(row.progress || 0);
    const value = row.heartbeat_count > 0 ? `${Math.round(progress * 100)}%` : "-";
    addOpsRow(fragment, title, detail, value, runLivenessSeverity(row));
  });
  const remainingSlots = Math.max(0, 8 - Math.min(runs.length, 4));
  events.slice(0, remainingSlots).forEach((row) => {
    addOpsRow(fragment, row.kind || "event", `${row.source || "-"} · ${row.message || ""}`, fmt(row.value || 0), row.severity || "ok");
  });
  if (events.length === 0 && runs.length === 0) fragment.appendChild(createMessage(t("noIssues"), "ops-empty"));
  const staleRuns = runs.filter((row) => row.stale).length;
  setText("watchdog-meta", `${events.length} events · ${runs.length} ${t("activeRuns")} · ${staleRuns} ${t("staleRuns")}`);
  list.replaceChildren(fragment);
}

function renderAuditLog(rows) {
  const list = $("audit-list");
  if (!list) return;
  const fragment = document.createDocumentFragment();
  (rows || []).slice(0, 8).forEach((row) => {
    const action = String(row.action || "-");
    const noisy = action.includes("error") || action.includes("blocked") || action.includes("rejected") || action.includes("failed");
    const detail = `${row.role || "-"} · ${row.target || "-"} · ${relTime(row.created_at)}`;
    addOpsRow(fragment, action, detail, row.actor || "-", noisy ? "warning" : "ok");
  });
  if (!rows || rows.length === 0) fragment.appendChild(createMessage(t("noAuditEvents"), "ops-empty"));
  setText("audit-meta", `${(rows || []).length} events`);
  list.replaceChildren(fragment);
}

function renderFleetAttribution(report) {
  const list = $("fleet-list");
  if (!list) return;
  const fragment = document.createDocumentFragment();
  const rows = (report && report.rows) || [];
  if (rows.length === 0) {
    fragment.appendChild(createMessage(t("noFleet"), "ops-empty"));
    setText("fleet-meta", "0");
    list.replaceChildren(fragment);
    return;
  }
  rows.slice(0, 8).forEach((row) => {
    const label = row.attribution || "run";
    const detail = `${row.source || "-"} · ${row.agent_name || "-"} · ${fmt(row.model_calls || 0)} ${t("calls")} · ${fmt(row.concurrent_runs || 1)}x`;
    const severity = row.attribution === "sub-agent" || Number(row.concurrent_runs || 1) > 1 ? "warning" : "ok";
    addOpsRow(fragment, `${label} · ${row.project || row.repo || row.workload_id || "-"}`, detail, `${fmt(row.tokens || 0)} · ${fmtCost(row.cost_usd || 0)}`, severity);
  });
  setText("fleet-meta", `${report.runs || rows.length} ${t("runs")} · max ${report.max_concurrent_runs || 1}x · ${fmtCost(report.cost_usd || 0)}`);
  list.replaceChildren(fragment);
}

function renderReconciliation(rows) {
  const list = $("reconciliation-list");
  if (!list) return;
  const fragment = document.createDocumentFragment();
  const imports = rows || [];
  if (imports.length === 0) {
    fragment.appendChild(createMessage(t("noReconciliation"), "ops-empty"));
    setText("s-reconciliation", "-");
    setText("s-reconciliation-sub", t("noReconciliation"));
    setText("reconciliation-meta", "0");
    list.replaceChildren(fragment);
    return;
  }
  const severityFor = (status) => {
    if (status === "mismatch" || status === "empty") return "warning";
    if (status === "warning") return "warning";
    return "ok";
  };
  const latest = imports[0];
  const mismatches = imports.filter((row) => row.status === "mismatch" || row.status === "empty").length;
  imports.slice(0, 8).forEach((row) => {
    const hash = row.payload_sha256 ? ` · ${String(row.payload_sha256).slice(0, 12)}` : "";
    const detail = `${t("localLedger")} ${fmtCost(row.local_cost_usd)} · ${t("providerBill")} ${fmtCost(row.provider_cost_usd)}${hash}`;
    addOpsRow(fragment, `${row.provider || "provider"} · ${row.format || "-"}`, detail, fmtSignedCost(row.diff_usd || 0), severityFor(row.status));
  });
  setText("s-reconciliation", latest.status ? latest.status.toUpperCase() : "OK");
  setText("s-reconciliation-sub", `${latest.provider || "provider"} · ${fmtSignedCost(latest.diff_usd || 0)}`);
  setText("reconciliation-meta", mismatches ? `${mismatches} mismatch` : `${imports.length} imports`);
  list.replaceChildren(fragment);
}

function renderChargeback(rows) {
  const list = $("chargeback-list");
  if (!list) return;
  const fragment = document.createDocumentFragment();
  const data = rows || [];
  if (data.length === 0) {
    fragment.appendChild(createMessage(t("noChargeback"), "ops-empty"));
    setText("chargeback-meta", "0");
    list.replaceChildren(fragment);
    return;
  }
  const totalCost = data.reduce((sum, row) => sum + Number(row.cost_usd || 0), 0);
  const teams = new Set(data.map((row) => row.team || "unassigned"));
  data.slice(0, 8).forEach((row) => {
    const team = row.team || "unassigned";
    const detail = `${row.project || "-"} · ${row.source || "-"} / ${row.model || t("unknownModel")} · ${fmt(row.sessions || 0)} sessions · ${row.mapping_source || "unmapped"}`;
    addOpsRow(fragment, team, detail, fmtCost(row.cost_usd || 0), row.team === "unassigned" || row.unpriced_calls ? "warning" : "ok");
  });
  setText("chargeback-meta", `${teams.size} ${t("team")} · ${fmtCost(totalCost)}`);
  list.replaceChildren(fragment);
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
      inRange: { color: ["#1a1a1a", "#575757", "#a3a3a3", "#f5f5f5"] },
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
    const sessionOffset = (sessionPage - 1) * PAGE_SIZE;
    const sessionParams = {
      limit: PAGE_SIZE,
      offset: sessionOffset,
      sort: sessionSort.key,
      dir: sessionSort.dir,
    };
    if (state.ledgerQuery) sessionParams.q = state.ledgerQuery;
    const workloadParams = {
      limit: PAGE_SIZE,
      offset: 0,
    };
    if (state.ledgerQuery) workloadParams.q = state.ledgerQuery;
    const requests = {
      dashboard: api("dashboard"),
      workloads: api("workloads", { extra: workloadParams }),
      sessions: api("sessions", { extra: sessionParams }),
      health: api("health/ingestion"),
      budgets: api("budgets/status"),
      quota: api("quota/status"),
      pricing: api("pricing/status"),
      quality: api("data-quality"),
      policyAudit: api("policy/audit", { extra: { limit: 20 } }),
      modelCalls: api("model-calls", { skipModel: true }),
      costIntel: api("cost-intelligence"),
      cacheDoctor: api("cache/doctor"),
      watchdog: api("watchdog/events", { skipModel: true }),
      auditLog: api("audit-log", { extra: { limit: 20 } }),
      liveness: api("agent-runs/liveness", { skipModel: true, extra: { max_age: "10m", limit: 20 } }),
      fleet: api("fleet-attribution", { extra: { limit: 50 } }),
      reconciliation: api("reconciliation/status", { skipModel: true, extra: { limit: 20 } }),
      chargeback: api("chargeback", { extra: { limit: 50 } }),
    };
    const settled = await Promise.allSettled(Object.entries(requests).map(async ([key, promise]) => [key, await promise]));
    const data = {};
    const errors = [];
    let consistencyWarnings = 0;
    settled.forEach((result) => {
      if (result.status === "fulfilled") {
        data[result.value[0]] = result.value[1];
      } else {
        errors.push(result.reason && result.reason.message ? result.reason.message : String(result.reason));
      }
    });

    if (data.dashboard) {
      applyRuntimeStatus(data.dashboard.runtime);
      data.stats = data.dashboard.stats;
      data.costModel = data.dashboard.cost_by_model || [];
      data.costTime = data.dashboard.cost_over_time || [];
      data.tokensTime = data.dashboard.tokens_over_time || [];
      const consistency = data.dashboard.consistency || [];
      consistencyWarnings = consistency.length;
      if (consistencyWarnings > 0 && !options.silent) {
        console.warn("Agent Ledger dashboard consistency warnings", consistency);
        showStatus(`${consistencyWarnings} dashboard consistency warning(s)`, "error");
      }
    }

    const costModel = data.costModel || [];
    if (data.costModel) updateModelFilter(costModel);
    if (data.stats) renderStats(data.stats);
    const modelColorMap = buildModelColorMap(costModel);
    if (data.tokensTime) {
      renderActivityMatrix(data.tokensTime);
      renderTokenThroughput(data.tokensTime);
    }
    if (data.costTime) renderCostTrend(data.costTime, modelColorMap);
    if (data.costModel) renderModelAllocation(costModel, modelColorMap);
    if (data.health) renderHealth(data.health);
    if (data.budgets) renderBudgets(data.budgets);
    if (data.quota) renderQuota(data.quota);
    if (data.pricing) renderPricing(data.pricing);
    if (data.quality || data.policyAudit) renderQuality(data.quality, data.policyAudit);
    if (data.modelCalls) renderModelCalls(data.modelCalls);
    if (data.costIntel) renderCostIntelligence(data.costIntel);
    if (data.cacheDoctor) renderCacheDoctor(data.cacheDoctor);
    if (data.watchdog || data.liveness) renderWatchdog(data.watchdog, data.liveness);
    if (data.auditLog) renderAuditLog(data.auditLog);
    if (data.fleet) renderFleetAttribution(data.fleet);
    if (data.reconciliation) renderReconciliation(data.reconciliation);
    if (data.chargeback) renderChargeback(data.chargeback);
    if (data.workloads) {
      allWorkloads = data.workloads.rows || [];
      workloadTotal = Number(data.workloads.total || allWorkloads.length);
      renderWorkloadTable();
    }
    if (data.sessions) {
      allSessions = data.sessions.rows || [];
      sessionTotal = Number(data.sessions.total || allSessions.length);
      renderSessionTable();
    }
    if (errors.length > 0) {
      showStatus(`${t("partialRefreshFailed")}: ${errors.slice(0, 2).join("; ")}`, "error");
    } else if (!options.silent && consistencyWarnings === 0) {
      showStatus(`${t("updated")} ${new Date().toLocaleTimeString()}`);
    }
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

function renderWorkloadTable() {
  const tbody = $("workload-table");
  if (!tbody) return;
  const fragment = document.createDocumentFragment();
  if (allWorkloads.length === 0) {
    const tr = document.createElement("tr");
    const td = createCell(t("noData"), "empty-state");
    td.colSpan = 10;
    tr.appendChild(td);
    fragment.appendChild(tr);
  } else {
    allWorkloads.forEach((workload) => {
      const id = workload.workload_id || "";
      const isExpanded = expandedWorkloads.has(id);
      const tr = document.createElement("tr");
      tr.className = `workload-row${isExpanded ? " expanded" : ""}`;
      const goal = createCell(workload.goal || id || "-", "project-cell");
      goal.title = id;
      tr.appendChild(goal);
      tr.appendChild(createCell(workload.status || "-", "muted-cell"));
      tr.appendChild(createSourceCell(workload.source || "-"));
      tr.appendChild(createCell(workload.project || workload.repo || "-", "project-cell"));
      tr.appendChild(createCell(workload.git_branch || "-", "muted-cell"));
      tr.appendChild(createCell(fmt(workload.model_calls || 0), "num"));
      tr.appendChild(createCell(fmt(workload.tokens || 0), "num"));
      tr.appendChild(createCell(fmtCost(workload.cost_usd || 0), "cost-cell"));
      tr.appendChild(createCell(workload.outcome || "-", "muted-cell"));

      const expandCell = document.createElement("td");
      const button = document.createElement("button");
      button.type = "button";
      button.className = `expand-btn${isExpanded ? " open" : ""}`;
      button.dataset.workloadId = id;
      button.setAttribute("aria-expanded", isExpanded ? "true" : "false");
      button.setAttribute("aria-label", `${t("workloadLedger")} ${id}`);
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
        fetchAndFillWorkloadDetail(detail.content, id);
      }
    });
  }
  tbody.replaceChildren(fragment);
  setText("workload-meta", `${workloadTotal} ${t("rows")}`);
}

function pct(value) {
  const n = Math.max(0, Math.min(1, Number(value || 0)));
  return `${Math.round(n * 100)}%`;
}

function buildWorkloadStatePanel(snapshot) {
  if (!snapshot || !snapshot.workload_id) return null;
  const panel = document.createElement("section");
  panel.className = "workload-state-panel";
  panel.setAttribute("aria-label", t("terminalState"));

  const header = document.createElement("div");
  header.className = "workload-state-head";
  const title = document.createElement("div");
  title.className = "workload-state-title";
  const label = document.createElement("span");
  label.textContent = t("terminalState");
  const phase = document.createElement("strong");
  phase.textContent = snapshot.phase || snapshot.status || "-";
  title.append(label, phase);
  const status = document.createElement("span");
  status.className = "state-pill";
  status.textContent = snapshot.terminal ? t("terminal") : (snapshot.status || "-");
  header.append(title, status);
  panel.appendChild(header);

  const metrics = document.createElement("div");
  metrics.className = "workload-state-metrics";
  [
    [t("readiness"), pct(snapshot.readiness_score)],
    [t("progress"), pct(snapshot.progress)],
    [t("activeRuns"), fmt(snapshot.active_runs || 0)],
    [t("staleRuns"), fmt(snapshot.stale_runs || 0)],
  ].forEach(([key, value]) => {
    const item = document.createElement("div");
    item.className = "state-metric";
    const k = document.createElement("span");
    k.textContent = key;
    const v = document.createElement("strong");
    v.textContent = value;
    item.append(k, v);
    metrics.appendChild(item);
  });
  panel.appendChild(metrics);

  const next = document.createElement("div");
  next.className = "state-next";
  next.textContent = `${t("nextAction")}: ${snapshot.next_action || "-"}`;
  panel.appendChild(next);

  const notes = document.createElement("div");
  notes.className = "state-notes";
  const reasons = Array.isArray(snapshot.reasons) && snapshot.reasons.length ? snapshot.reasons.slice(0, 3).join(" · ") : "-";
  const risks = Array.isArray(snapshot.risks) && snapshot.risks.length ? snapshot.risks.slice(0, 3).join(" · ") : t("noRisks");
  const reasonEl = document.createElement("span");
  reasonEl.textContent = `${t("reasons")}: ${reasons}`;
  const riskEl = document.createElement("span");
  riskEl.textContent = `${t("risks")}: ${risks}`;
  notes.append(reasonEl, riskEl);
  panel.appendChild(notes);
  return panel;
}

function buildWorkloadDetail(data, timelineRows = [], workloadState = null) {
  const wrap = document.createElement("div");
  wrap.className = "workload-detail-grid";
  const summary = data.summary || {};
  const statePanel = buildWorkloadStatePanel(workloadState);
  if (statePanel) wrap.appendChild(statePanel);
  const facts = [
    [t("runs"), summary.runs || 0],
    [t("modelCalls"), summary.model_calls || 0],
    [t("toolCalls"), summary.tool_calls || 0],
    [t("contextRefs"), Array.isArray(data.context_refs) ? data.context_refs.length : 0],
    [t("tokens"), fmt(summary.tokens || 0)],
    [t("cost"), fmtCost(summary.cost_usd || 0)],
    [t("confidence"), `${Math.round(Number(summary.confidence || 0) * 100)}%`],
  ];
  facts.forEach(([label, value]) => {
    const item = document.createElement("div");
    item.className = "detail-metric";
    const k = document.createElement("span");
    k.textContent = label;
    const v = document.createElement("strong");
    v.textContent = String(value);
    item.append(k, v);
    wrap.appendChild(item);
  });
  const appendDetailTable = (headers, rows) => {
    if (!rows.length) return;
    const table = document.createElement("table");
    table.className = "detail-table";
    const head = document.createElement("thead");
    const headRow = document.createElement("tr");
    headers.forEach((label) => {
      const th = document.createElement("th");
      th.textContent = label;
      headRow.appendChild(th);
    });
    head.appendChild(headRow);
    table.appendChild(head);
    const body = document.createElement("tbody");
    rows.forEach((cells) => body.appendChild(cells));
    table.appendChild(body);
    wrap.appendChild(table);
  };
  const calls = Array.isArray(data.model_calls) ? data.model_calls.slice(0, 8) : [];
  appendDetailTable([t("model"), t("source"), t("calls"), t("tokens"), t("cost")], calls.map((row) => {
    const tr = document.createElement("tr");
    tr.appendChild(createCell(row.model || t("unknownModel"), "project-cell"));
    tr.appendChild(createCell(row.source || "-", "muted-cell"));
    tr.appendChild(createCell(fmt(row.calls || 0), "num"));
    tr.appendChild(createCell(fmt(row.tokens || 0), "num"));
    tr.appendChild(createCell(fmtCost(row.cost_usd || 0), "cost-cell"));
    return tr;
  }));
  const tools = Array.isArray(data.tool_calls) ? data.tool_calls.slice(0, 8) : [];
  appendDetailTable([t("tool"), t("refType"), t("status"), t("duration"), t("source")], tools.map((row) => {
    const tr = document.createElement("tr");
    tr.appendChild(createCell(row.tool_name || "-", "project-cell"));
    tr.appendChild(createCell(row.tool_type || "-", "muted-cell"));
    tr.appendChild(createCell(row.status || "-", "muted-cell"));
    tr.appendChild(createCell(`${fmt(row.duration_ms || 0)} ms`, "num"));
    tr.appendChild(createCell(row.source || "-", "muted-cell"));
    return tr;
  }));
  const contexts = Array.isArray(data.context_refs) ? data.context_refs.slice(0, 8) : [];
  appendDetailTable([t("contextRefs"), t("refType"), t("project"), t("branch"), t("privacy"), t("refHash")], contexts.map((row) => {
    const tr = document.createElement("tr");
    tr.appendChild(createCell(row.label || row.context_ref_id || "-", "project-cell"));
    tr.appendChild(createCell(row.ref_type || "-", "muted-cell"));
    tr.appendChild(createCell(row.repo || "-", "project-cell"));
    tr.appendChild(createCell(row.git_branch || "-", "muted-cell"));
    tr.appendChild(createCell(row.privacy_label || "-", "muted-cell"));
    tr.appendChild(createCell(row.ref_hash || "-", "muted-cell"));
    return tr;
  }));
  const timeline = Array.isArray(timelineRows) ? timelineRows.slice(-12) : [];
  appendDetailTable([t("timeline"), t("time"), t("status"), t("tokens"), t("cost")], timeline.map((row) => {
    const tr = document.createElement("tr");
    const label = `${row.kind || "-"} · ${row.label || row.id || "-"}`;
    tr.appendChild(createCell(label, "project-cell"));
    tr.appendChild(createCell(relTime(row.timestamp), "muted-cell"));
    tr.appendChild(createCell(row.status || "-", "muted-cell"));
    tr.appendChild(createCell(fmt(row.tokens || 0), "num"));
    tr.appendChild(createCell(fmtCost(row.cost_usd || 0), "cost-cell"));
    return tr;
  }));
  return wrap;
}

async function fetchAndFillWorkloadDetail(content, workloadID) {
  if (!workloadID) {
    content.replaceChildren(createMessage(t("noDetails"), "empty-state"));
    return;
  }
  try {
    const params = new URLSearchParams({ workload_id: workloadID });
    if (state.privacy) params.set("privacy", "1");
    const res = await fetch(`/api/workload-detail?${params.toString()}`);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = await res.json();
    let timeline = [];
    let workloadState = null;
    try {
      const timelineParams = new URLSearchParams({ workload_id: workloadID, limit: "50" });
      if (state.privacy) timelineParams.set("privacy", "1");
      const timelineRes = await fetch(`/api/workload-timeline?${timelineParams.toString()}`);
      if (timelineRes.ok) {
        const timelineData = await timelineRes.json();
        timeline = Array.isArray(timelineData.rows) ? timelineData.rows : [];
      }
    } catch (err) {
      timeline = [];
    }
    try {
      const stateParams = new URLSearchParams({ workload_id: workloadID, max_age: "10m" });
      if (state.privacy) stateParams.set("privacy", "1");
      const stateRes = await fetch(`/api/workload-state?${stateParams.toString()}`);
      if (stateRes.ok) workloadState = await stateRes.json();
    } catch (err) {
      workloadState = null;
    }
    content.replaceChildren(buildWorkloadDetail(data, timeline, workloadState));
  } catch (err) {
    content.replaceChildren(createMessage(`${t("detailFailed")} ${err.message}`, "empty-state"));
  }
}

function renderSessionTable() {
  const totalPages = Math.max(1, Math.ceil(sessionTotal / PAGE_SIZE));
  if (sessionPage > totalPages) sessionPage = totalPages;
  const start = (sessionPage - 1) * PAGE_SIZE;
  const page = allSessions;
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
      const expandKey = `${session.source || ""}\u0000${sid}`;
      sessionKeyToID.set(key, { sid, source: session.source || "" });
      const isExpanded = expandedSessions.has(expandKey);
      const tr = document.createElement("tr");
      tr.className = `session-row${isExpanded ? " expanded" : ""}`;

      tr.appendChild(createSourceCell(session.source));
      const projectCell = createCell(session.project || session.cwd || "-", "project-cell");
      projectCell.title = session.cwd || session.project || "";
      tr.appendChild(projectCell);
      tr.appendChild(createCell(session.git_branch || "-", "muted-cell"));
      const displayTime = session.last_activity || session.start_time;
      const timeCell = createCell(relTime(displayTime), "mono");
      timeCell.title = `${fmtLocalTime(displayTime)}${session.start_time && session.start_time !== displayTime ? ` · start ${fmtLocalTime(session.start_time)}` : ""}`;
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
      button.disabled = state.privacy;
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
        fetchAndFillDetail(detail.content, sid, session.source || "");
      }
    });
  }

  tbody.replaceChildren(fragment);
  setText("ledger-meta", `${sessionTotal} ${t("rows")}`);
  renderPagination(sessionTotal, start, totalPages);
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

async function fetchAndFillDetail(content, sid, source) {
  if (!sid) {
    content.replaceChildren(createMessage(t("noDetails"), "empty-state"));
    return;
  }
  try {
    const params = new URLSearchParams({ session_id: sid });
    if (source) params.set("source", source);
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
  document.title = PRODUCT_NAME;
  document.querySelectorAll("[data-i18n]").forEach((el) => {
    el.textContent = t(el.dataset.i18n);
  });
  setText("product-name", PRODUCT_NAME);

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
  $("filter-project").placeholder = t("ledgerSearch");
  $("filter-project").value = state.ledgerQuery;
  $("filter-project-global").placeholder = t("filterProject");
  $("filter-project-global").value = state.project;
  $("privacy-status").textContent = state.privacy ? t("privacyOn") : t("privacyOff");
  $("btn-privacy").classList.toggle("active", state.privacy);
  $("btn-reset-scan").disabled = !state.source;
  updateRangeCaption();
  applyRuntimeStatus(state.runtime);
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
  $("btn-reset-scan").disabled = !state.source;
  refresh();
});

$("filter-model").addEventListener("change", (e) => {
  persist("model", e.target.value);
  sessionPage = 1;
  refresh();
});

$("filter-project-global").addEventListener("input", (e) => {
  persist("project", e.target.value.trim());
  sessionPage = 1;
  if (projectFilterTimer) clearTimeout(projectFilterTimer);
  projectFilterTimer = setTimeout(() => refresh(), 400);
});

$("filter-project").addEventListener("input", () => {
  persist("ledgerQuery", $("filter-project").value.trim());
  sessionPage = 1;
  if (projectFilterTimer) clearTimeout(projectFilterTimer);
  projectFilterTimer = setTimeout(() => refresh({ silent: true }), 300);
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

$("btn-scan").addEventListener("click", async () => {
  try {
    showStatus(t("scanStarted"));
    await postApi("scan", { extra: state.source ? { source: state.source } : {} });
    await refresh();
    showStatus(t("scanDone"));
  } catch (err) {
    showStatus(`${t("actionFailed")}: ${err.message}`, "error");
  }
});

$("btn-pricing-sync").addEventListener("click", async () => {
  try {
    await postApi("pricing/sync");
    await postApi("pricing/recalculate", { extra: { mode: "zero" } });
    await refresh();
    showStatus(t("pricingDone"));
  } catch (err) {
    showStatus(`${t("actionFailed")}: ${err.message}`, "error");
  }
});

$("btn-doctor").addEventListener("click", () => {
  downloadApi("doctor", { extra: { format: "markdown", privacy: state.privacy ? "1" : "" } });
});

$("btn-recalc").addEventListener("click", async () => {
  try {
    await postApi("recalculate-costs");
    await refresh();
    showStatus(t("recalcDone"));
  } catch (err) {
    showStatus(`${t("actionFailed")}: ${err.message}`, "error");
  }
});

$("btn-repair-projections").addEventListener("click", async () => {
  try {
    showStatus(t("repairStarted"));
    const body = await postApi("projections/repair");
    await refresh();
    const result = body.result || {};
    showStatus(`${t("repairDone")}: ${fmt(result.inserted || 0)} ${t("inserted")}, ${fmt(result.updated || 0)} ${t("updatedRows")}`);
  } catch (err) {
    showStatus(`${t("actionFailed")}: ${err.message}`, "error");
  }
});

$("btn-reset-scan").addEventListener("click", async () => {
  if (!state.source) {
    showStatus(t("resetNeedsSource"), "error");
    return;
  }
  if (!window.confirm(t("resetConfirm"))) return;
  try {
    showStatus(t("scanStarted"));
    await postApi("scan", { extra: { source: state.source, reset: "true" } });
    sessionPage = 1;
    expandedSessions.clear();
    await refresh();
    showStatus(t("scanDone"));
  } catch (err) {
    showStatus(`${t("actionFailed")}: ${err.message}`, "error");
  }
});

$("btn-export").addEventListener("click", () => {
  downloadApi("export", { extra: { type: "workloads", format: "csv", limit: 10000 } });
});

$("btn-report").addEventListener("click", () => {
  downloadApi("report", { extra: { format: "markdown" } });
});

$("btn-privacy").addEventListener("click", () => {
  persist("privacy", !state.privacy);
  expandedWorkloads.clear();
  expandedSessions.clear();
  buildControls();
  refresh();
});

$("btn-auto-refresh").addEventListener("click", () => {
  persist("autoRefresh", !state.autoRefresh);
  applyAutoRefresh();
});

$("session-table").addEventListener("click", (e) => {
  const button = e.target.closest(".expand-btn");
  if (!button) return;
  const ref = sessionKeyToID.get(button.dataset.sessionKey) || {};
  const sid = ref.sid || "";
  const source = ref.source || "";
  const expandKey = `${source}\u0000${sid}`;
  const row = button.closest(".session-row");
  const next = row ? row.nextElementSibling : null;
  if (expandedSessions.has(expandKey)) {
    expandedSessions.delete(expandKey);
    button.classList.remove("open");
    button.setAttribute("aria-expanded", "false");
    if (row) row.classList.remove("expanded");
    if (next && next.classList.contains("detail-row")) next.remove();
    return;
  }

  expandedSessions.add(expandKey);
  button.classList.add("open");
  button.setAttribute("aria-expanded", "true");
  if (row) {
    row.classList.add("expanded");
    const detail = buildDetailShell();
    row.after(detail.row);
    fetchAndFillDetail(detail.content, sid, source);
  }
});

$("workload-table").addEventListener("click", (e) => {
  const button = e.target.closest(".expand-btn");
  if (!button) return;
  const workloadID = button.dataset.workloadId || "";
  const row = button.closest(".workload-row");
  const next = row ? row.nextElementSibling : null;
  if (expandedWorkloads.has(workloadID)) {
    expandedWorkloads.delete(workloadID);
    button.classList.remove("open");
    button.setAttribute("aria-expanded", "false");
    if (row) row.classList.remove("expanded");
    if (next && next.classList.contains("detail-row")) next.remove();
    return;
  }
  expandedWorkloads.add(workloadID);
  button.classList.add("open");
  button.setAttribute("aria-expanded", "true");
  if (row) {
    row.classList.add("expanded");
    const detail = buildDetailShell();
    row.after(detail.row);
    fetchAndFillWorkloadDetail(detail.content, workloadID);
  }
});

$("pagination").addEventListener("click", (e) => {
  const button = e.target.closest(".page-btn:not(:disabled)");
  if (!button || button.classList.contains("active")) return;
  sessionPage = Number(button.dataset.page);
  refresh({ silent: true });
});

document.querySelectorAll(".sort-button").forEach((button) => {
  button.addEventListener("click", () => {
    const key = button.dataset.sort;
    if (sessionSort.key === key) {
      sessionSort.dir = sessionSort.dir === "asc" ? "desc" : "asc";
    } else {
      sessionSort.key = key;
      sessionSort.dir = ["start_time", "last_activity", "total_cost", "tokens", "prompts"].includes(key) ? "desc" : "asc";
    }
    sessionPage = 1;
    refresh({ silent: true });
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
