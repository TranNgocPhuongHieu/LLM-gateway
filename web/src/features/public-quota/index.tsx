import { useEffect, useMemo, useState } from "react";
import {
  Check,
  ChevronDown,
  Eye,
  EyeOff,
  KeyRound,
  Moon,
  RefreshCw,
  Search,
  Sun,
  Trash2,
} from "lucide-react";

import { getPublicQuota, type UsageLog, type TokenUsage } from "./api";

const STORAGE_KEY = "llm_trnhieu_quota_key_v1";
const number = new Intl.NumberFormat("vi-VN");
const date = new Intl.DateTimeFormat("vi-VN", {
  dateStyle: "medium",
  timeStyle: "short",
});

function formatNumber(value: number) {
  return number.format(Math.max(0, Math.round(Number(value) || 0)));
}

function expiry(value: number) {
  if (!value) return "Không giới hạn";
  const timestamp = value > 10_000_000_000 ? value : value * 1000;
  return date.format(new Date(timestamp));
}

function summarize(logs: UsageLog[]) {
  const byModel = new Map<
    string,
    { requests: number; input: number; output: number; quota: number }
  >();
  for (const log of logs) {
    const model = log.model_name || "Không rõ model";
    const current = byModel.get(model) ?? { requests: 0, input: 0, output: 0, quota: 0 };
    current.requests += 1;
    current.input += Number(log.prompt_tokens) || 0;
    current.output += Number(log.completion_tokens) || 0;
    current.quota += Number(log.quota) || 0;
    byModel.set(model, current);
  }
  return [...byModel.entries()].sort((a, b) => b[1].requests - a[1].requests);
}

function Stat({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="rounded-2xl border border-slate-200/80 bg-white/70 p-5 shadow-sm dark:border-slate-800 dark:bg-slate-900/70">
      <div className="text-xs font-semibold uppercase tracking-[0.14em] text-slate-500 dark:text-slate-400">
        {label}
      </div>
      <div className="mt-2 text-2xl font-bold tabular-nums text-slate-950 dark:text-white">
        {value}
      </div>
      {hint && <div className="mt-1 text-xs text-slate-500 dark:text-slate-400">{hint}</div>}
    </div>
  );
}

function QuotaDashboard({
  usage,
  logs,
  onRefresh,
}: {
  usage: TokenUsage;
  logs: UsageLog[];
  onRefresh: () => void;
}) {
  const [model, setModel] = useState("");
  const rows = useMemo(
    () => logs.filter((row) => !model || row.model_name === model),
    [logs, model],
  );
  const models = useMemo(() => summarize(logs), [logs]);
  const percent =
    usage.total_granted > 0 ? Math.min(100, (usage.total_used / usage.total_granted) * 100) : 0;
  const remaining = usage.unlimited_quota ? "Không giới hạn" : formatNumber(usage.total_available);
  return (
    <div className="mx-auto w-full max-w-6xl space-y-6">
      <section className="rounded-3xl border border-slate-200/80 bg-white/75 p-6 shadow-sm backdrop-blur dark:border-slate-800 dark:bg-slate-900/75">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <div className="font-mono text-sm text-blue-600 dark:text-blue-400">
              @{usage.name || "API key"}
            </div>
            <h2 className="mt-2 text-3xl font-bold tracking-tight text-slate-950 dark:text-white">
              Quota API key
            </h2>
            <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
              {usage.quota_mode === "token"
                ? "Key token: input + output + cache được tính một lần."
                : "Số liệu quota billing của NewAPI."}
            </p>
          </div>
          <button
            onClick={onRefresh}
            className="inline-flex h-10 items-center gap-2 rounded-xl border border-slate-200 px-3 text-sm font-semibold hover:border-blue-400 dark:border-slate-700"
          >
            <RefreshCw className="size-4" /> Làm mới
          </button>
        </div>
        <div className="mt-8 grid gap-5 md:grid-cols-[1.3fr_1fr]">
          <div>
            <div className="flex items-end justify-between gap-3">
              <div className="text-4xl font-bold tabular-nums text-slate-950 dark:text-white">
                {remaining}
              </div>
              <div className="text-sm text-slate-500">đơn vị quota billing còn lại</div>
            </div>
            <div className="mt-4 h-3 overflow-hidden rounded-full bg-slate-200 dark:bg-slate-800">
              <div
                className="h-full rounded-full bg-gradient-to-r from-blue-500 to-cyan-400"
                style={{ width: `${usage.unlimited_quota ? 0 : Math.max(0, 100 - percent)}%` }}
              />
            </div>
            <div className="mt-2 flex justify-between text-xs text-slate-500">
              <span>
                {usage.unlimited_quota
                  ? "Quota không giới hạn"
                  : `${formatNumber(usage.total_used)} đã dùng`}
              </span>
              <span>{usage.unlimited_quota ? "∞" : `${percent.toFixed(1)}% đã dùng`}</span>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <Stat
              label={usage.quota_mode === "token" ? "Tổng token" : "Tổng quota billing"}
              value={usage.unlimited_quota ? "∞" : formatNumber(usage.total_granted)}
              hint={
                usage.quota_mode === "token" ? "Raw token ledger" : "Không phải số token thực tế"
              }
            />
            <Stat label="Hết hạn" value={expiry(usage.expires_at)} />
          </div>
        </div>
      </section>

      <div className="grid gap-4 md:grid-cols-4">
        <Stat label="Request" value={formatNumber(rows.length)} />
        <Stat
          label="Input token"
          value={formatNumber(rows.reduce((sum, row) => sum + (Number(row.prompt_tokens) || 0), 0))}
        />
        <Stat
          label="Output token"
          value={formatNumber(
            rows.reduce((sum, row) => sum + (Number(row.completion_tokens) || 0), 0),
          )}
        />
        <Stat
          label={usage.quota_mode === "token" ? "Token đã dùng" : "Quota billing đã dùng"}
          value={
            usage.quota_mode === "token"
              ? formatNumber(usage.total_used)
              : formatNumber(rows.reduce((sum, row) => sum + (Number(row.quota) || 0), 0))
          }
        />
      </div>

      <section className="rounded-3xl border border-slate-200/80 bg-white/75 p-6 shadow-sm backdrop-blur dark:border-slate-800 dark:bg-slate-900/75">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 className="text-xl font-bold text-slate-950 dark:text-white">Theo model</h3>
            <p className="text-sm text-slate-500">Tổng hợp trong phạm vi log gần nhất của key.</p>
          </div>
          <select
            value={model}
            onChange={(event) => setModel(event.target.value)}
            className="h-10 rounded-xl border border-slate-200 bg-transparent px-3 text-sm dark:border-slate-700"
          >
            <option value="">Tất cả model</option>
            {models.map(([name]) => (
              <option key={name} value={name}>
                {name}
              </option>
            ))}
          </select>
        </div>
        <div className="mt-5 overflow-x-auto">
          <table className="w-full min-w-[620px] text-left text-sm">
            <thead className="border-b border-slate-200 text-xs uppercase tracking-wide text-slate-500 dark:border-slate-800">
              <tr>
                <th className="px-3 py-3">Model</th>
                <th className="px-3 py-3 text-right">Request</th>
                <th className="px-3 py-3 text-right">Input</th>
                <th className="px-3 py-3 text-right">Output</th>
                <th className="px-3 py-3 text-right">Quota</th>
              </tr>
            </thead>
            <tbody>
              {models
                .filter(([name]) => !model || name === model)
                .map(([name, row]) => (
                  <tr
                    key={name}
                    className="border-b border-slate-100 last:border-0 dark:border-slate-800/70"
                  >
                    <td className="px-3 py-3 font-mono">{name}</td>
                    <td className="px-3 py-3 text-right">{formatNumber(row.requests)}</td>
                    <td className="px-3 py-3 text-right">{formatNumber(row.input)}</td>
                    <td className="px-3 py-3 text-right">{formatNumber(row.output)}</td>
                    <td className="px-3 py-3 text-right">{formatNumber(row.quota)}</td>
                  </tr>
                ))}
            </tbody>
          </table>
        </div>
      </section>

      <section className="rounded-3xl border border-slate-200/80 bg-white/75 p-6 shadow-sm backdrop-blur dark:border-slate-800 dark:bg-slate-900/75">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h3 className="text-xl font-bold text-slate-950 dark:text-white">Lịch sử request</h3>
            <p className="text-sm text-slate-500">Chỉ hiển thị hoạt động của API key này.</p>
          </div>
          <ChevronDown className="size-5 text-slate-400" />
        </div>
        <div className="mt-5 overflow-x-auto">
          <table className="w-full min-w-[760px] text-left text-sm">
            <thead className="border-b border-slate-200 text-xs uppercase tracking-wide text-slate-500 dark:border-slate-800">
              <tr>
                <th className="px-3 py-3">Thời gian</th>
                <th className="px-3 py-3">Model</th>
                <th className="px-3 py-3 text-right">Input</th>
                <th className="px-3 py-3 text-right">Output</th>
                <th className="px-3 py-3 text-right">Quota</th>
                <th className="px-3 py-3 text-right">Thời gian xử lý</th>
              </tr>
            </thead>
            <tbody>
              {rows.slice(0, 100).map((row) => (
                <tr
                  key={`${row.id}-${row.request_id ?? ""}`}
                  className="border-b border-slate-100 last:border-0 dark:border-slate-800/70"
                >
                  <td className="whitespace-nowrap px-3 py-3 text-slate-500">
                    {date.format(new Date(row.created_at * 1000))}
                  </td>
                  <td className="px-3 py-3 font-mono">{row.model_name || "—"}</td>
                  <td className="px-3 py-3 text-right">{formatNumber(row.prompt_tokens)}</td>
                  <td className="px-3 py-3 text-right">{formatNumber(row.completion_tokens)}</td>
                  <td className="px-3 py-3 text-right">{formatNumber(row.quota)}</td>
                  <td className="px-3 py-3 text-right">
                    {row.use_time ? `${row.use_time}s` : "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {rows.length === 0 && (
            <div className="py-10 text-center text-sm text-slate-500">Chưa có hoạt động.</div>
          )}
        </div>
      </section>
    </div>
  );
}

export function PublicQuotaPage() {
  const [key, setKey] = useState("");
  const [draft, setDraft] = useState("");
  const [showKey, setShowKey] = useState(false);
  const [dark, setDark] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [data, setData] = useState<{
    usage: TokenUsage;
    logs: UsageLog[];
    logsWarning?: string;
  } | null>(null);
  useEffect(() => {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved) {
      setDraft(saved);
      void lookup(saved);
    }
  }, []);
  async function lookup(nextKey = draft) {
    const clean = nextKey.trim();
    if (!clean) return;
    setLoading(true);
    setError("");
    try {
      const result = await getPublicQuota(clean);
      setKey(clean);
      setData(result);
      localStorage.setItem(STORAGE_KEY, clean);
    } catch (caughtError) {
      setData(null);
      setError(caughtError instanceof Error ? caughtError.message : "Không thể đọc quota API key.");
    } finally {
      setLoading(false);
    }
  }
  return (
    <div className={dark ? "dark" : ""}>
      <main className="min-h-svh bg-[radial-gradient(circle_at_top_left,_#dbeafe,_transparent_45%),#f8fafc] px-4 py-6 text-slate-900 dark:bg-[radial-gradient(circle_at_top_left,_#172554,_transparent_45%),#020617] dark:text-slate-100">
        <header className="mx-auto mb-14 flex max-w-6xl items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="grid size-10 place-items-center rounded-xl bg-blue-600 text-white shadow-lg">
              <KeyRound className="size-5" />
            </div>
            <div>
              <div className="font-bold">LLM</div>
              <div className="text-xs text-slate-500 dark:text-slate-400">API quota console</div>
            </div>
          </div>
          <button
            onClick={() => setDark((value) => !value)}
            className="rounded-xl border border-slate-200 p-2 dark:border-slate-700"
            aria-label="Đổi giao diện"
          >
            {dark ? <Sun className="size-4" /> : <Moon className="size-4" />}
          </button>
        </header>
        <section className={`mx-auto mb-8 w-full max-w-4xl text-center ${data ? "" : "pt-[12vh]"}`}>
          <div className="mb-3 inline-flex rounded-full bg-blue-100 px-3 py-1 text-xs font-semibold text-blue-700 dark:bg-blue-950 dark:text-blue-300">
            1B TOKEN API KEY
          </div>
          <h1 className="text-4xl font-extrabold tracking-tight text-slate-950 dark:text-white md:text-5xl">
            Kiểm tra quota API key
          </h1>
          <p className="mx-auto mt-4 max-w-xl text-sm leading-6 text-slate-500 dark:text-slate-400">
            Nhập API key bạn đã nhận để xem quota, billing usage và lịch sử request.
          </p>
          <form
            onSubmit={(event) => {
              event.preventDefault();
              void lookup();
            }}
            className="mx-auto mt-8 flex max-w-3xl gap-2 rounded-2xl border border-slate-200 bg-white/80 p-2 shadow-xl backdrop-blur dark:border-slate-800 dark:bg-slate-900/80"
          >
            <div className="relative flex-1">
              <input
                value={draft}
                onChange={(event) => setDraft(event.target.value)}
                type={showKey ? "text" : "password"}
                placeholder="sk-…"
                autoComplete="off"
                className="h-12 w-full rounded-xl border-0 bg-transparent px-3 pr-11 font-mono text-sm outline-none"
              />
              <button
                type="button"
                onClick={() => setShowKey((value) => !value)}
                className="absolute right-2 top-2 grid size-8 place-items-center rounded-lg text-slate-500"
                aria-label="Hiện hoặc ẩn key"
              >
                {showKey ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
              </button>
            </div>
            <button
              disabled={loading}
              className="inline-flex h-12 items-center gap-2 rounded-xl bg-blue-600 px-5 text-sm font-semibold text-white shadow-lg shadow-blue-600/20 disabled:opacity-60"
            >
              {loading ? (
                <RefreshCw className="size-4 animate-spin" />
              ) : (
                <Search className="size-4" />
              )}{" "}
              Kiểm tra
            </button>
          </form>
          <div className="mt-3 flex items-center justify-center gap-3 text-xs text-slate-500">
            <span>Key đang được lưu trên trình duyệt này.</span>
            {key && (
              <button
                onClick={() => {
                  localStorage.removeItem(STORAGE_KEY);
                  setKey("");
                  setDraft("");
                  setData(null);
                }}
                className="inline-flex items-center gap-1 text-red-500"
              >
                <Trash2 className="size-3" /> Quên key
              </button>
            )}
          </div>
          {error && (
            <div className="mx-auto mt-4 max-w-xl rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-950 dark:bg-red-950/40 dark:text-red-300">
              {error}
            </div>
          )}
        </section>
        {data && (
          <QuotaDashboard usage={data.usage} logs={data.logs} onRefresh={() => void lookup(key)} />
        )}
        <footer className="mx-auto mt-14 flex max-w-6xl items-center justify-center gap-2 text-xs text-slate-400">
          <Check className="size-3" /> Chỉ truy vấn dữ liệu read-only của key đã nhập
        </footer>
      </main>
    </div>
  );
}
