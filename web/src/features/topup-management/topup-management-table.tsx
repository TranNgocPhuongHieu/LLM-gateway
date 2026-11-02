import { ChevronLeft, ChevronRight, Copy, Search } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { getAllBillingHistory, isApiSuccess } from "@/features/wallet/api";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import type { TopupRecord } from "@/features/wallet/types";
import { api } from "@/lib/api";

const short = (v: string) => (v.length > 12 ? `${v.slice(0, 7)}…${v.slice(-4)}` : v);
export function formatUsdt(value: string, locale: string) {
  const compact = value.includes(".") ? value.replace(/0+$/, "").replace(/\.$/, "") : value;
  return locale.startsWith("vi") ? compact.replace(".", ",") : compact;
}
export const needsReview = (r: TopupRecord) =>
  r.status !== "success" &&
  ["review", "partial", "matched", "expired"].includes(r.binance?.status ?? "");

export function statusKey(r: TopupRecord) {
  if (r.status === "success") return "Successful";
  if (
    r.binance?.status === "voided" ||
    r.sepay?.verification_reason === "cancelled by user" ||
    r.sepay?.verification_reason === "cancelled by administrator"
  )
    return "Cancelled";
  if (r.binance?.status === "partial") return "Insufficient amount";
  if (r.binance?.status === "matched") return "Confirming network";
  if (r.binance?.status === "review" || r.sepay?.verification_status === "review")
    return "Manual review required";
  if (
    r.status === "expired" ||
    r.binance?.status === "expired" ||
    r.sepay?.verification_status === "voided"
  )
    return "Expired";
  return "Waiting for payment";
}

export async function loadTopups(signal?: AbortSignal): Promise<TopupRecord[]> {
  const records = new Map<number, TopupRecord>();
  // Existing API has no server-side method/TxHash filters. Bound aggregate
  // loading and fail explicitly instead of presenting partial global totals.
  for (let page = 1; page <= 100; page++) {
    signal?.throwIfAborted();
    const response = await getAllBillingHistory(page, 100);
    if (!isApiSuccess(response) || !response.data)
      throw new Error("Failed to load billing history");
    const { items, total } = response.data;
    if (total > 10000)
      throw new Error("History exceeds the display limit. Server-side filtering is required.");
    items.forEach((r) => records.set(r.id, r));
    if (page * 100 >= total) {
      if (records.size < total) throw new Error("History changed while loading. Please refresh.");
      return [...records.values()];
    }
    if (!items.length) throw new Error("Failed to load billing history");
  }
  throw new Error("History exceeds the display limit. Server-side filtering is required.");
}

export function TopupManagementTable() {
  const { t, i18n } = useTranslation();
  const locale = i18n.language.startsWith("vi") ? "vi-VN" : "en-US";
  const money = new Intl.NumberFormat(locale);
  const date = new Intl.DateTimeFormat(locale, {
    day: "2-digit",
    month: "2-digit",
    year: "2-digit",
  });
  const clock = new Intl.DateTimeFormat(locale, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
  const query = useQuery({
    queryKey: ["admin-topup-history-view"],
    queryFn: ({ signal }) => loadTopups(signal),
    retry: false,
    refetchOnWindowFocus: false,
  });
  const history = { records: query.data ?? [], refresh: query.refetch };
  const { copyToClipboard } = useCopyToClipboard();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [error, setError] = useState("");
  const [cfg, setCfg] = useState<{
    enabled: boolean;
    networks: Record<string, boolean>;
    auto_settlement_enabled: boolean;
    usdt_to_vnd: string;
    fee_percent: string;
  } | null>(null);
  const [q, setQ] = useState("");
  const [method, setMethod] = useState("all");
  const [status, setStatus] = useState("all");
  const [time, setTime] = useState("all");
  const [busy, setBusy] = useState("");
  const state = (r: TopupRecord) => [
    t(statusKey(r)),
    r.status === "success"
      ? "bg-success/10 text-success"
      : needsReview(r)
        ? "bg-warning/15 text-warning"
        : ["Expired", "Cancelled"].includes(statusKey(r))
          ? "bg-muted text-muted-foreground"
          : "bg-primary/10 text-primary",
  ];
  useEffect(() => {
    let active = true;
    void api
      .get("/api/option/binance")
      .then((r) => {
        if (active && r.data?.success) setCfg(r.data.data);
      })
      .catch(() => {
        if (active) setError(t("Unable to load USDT configuration."));
      });
    return () => {
      active = false;
    };
  }, [t]);
  useEffect(() => {
    setPage(1);
  }, [q, method, status, time, pageSize]);
  const rows = useMemo(() => {
    const now = Date.now() / 1000;
    const term = q.trim().toLowerCase();
    return history.records
      .filter(
        (r) =>
          (!term ||
            r.trade_no.toLowerCase().includes(term) ||
            (r.binance?.deposit?.tx_id ?? "").toLowerCase().includes(term)) &&
          (method === "all" ||
            (method === "binance" ? !!r.binance : r.payment_provider === "sepay")) &&
          (status === "all" || (status === "review" ? needsReview(r) : status === r.status)) &&
          (time === "all" ||
            r.create_time >=
              (time === "1"
                ? new Date(new Date().setHours(0, 0, 0, 0)).getTime() / 1000
                : now - Number(time) * 86400)),
      )
      .sort((a, b) => b.create_time - a.create_time);
  }, [history.records, q, method, status, time]);
  const manual = history.records.filter(needsReview).length;
  const pages = Math.max(1, Math.ceil(rows.length / pageSize));
  const current = Math.min(page, pages);
  const visible = rows.slice((current - 1) * pageSize, current * pageSize);
  const successfulVND = history.records
    .filter((r) => r.status === "success")
    .reduce((sum, r) => sum + (r.sepay?.amount_vnd ?? r.money), 0);
  const todayVND = history.records
    .filter(
      (r) =>
        r.status === "success" &&
        new Date(r.create_time * 1000).toDateString() === new Date().toDateString(),
    )
    .reduce((sum, r) => sum + (r.sepay?.amount_vnd ?? r.money), 0);
  async function action(trade: string, kind: "reconcile" | "review") {
    setBusy(trade);
    setError("");
    try {
      let response;
      if (kind === "review") {
        const reason = window.prompt(t("Review reason:"));
        if (!reason?.trim()) return;
        response = await api.post(`/api/user/binance/orders/${encodeURIComponent(trade)}/review`, {
          reason,
        });
      } else
        response = await api.post(
          `/api/user/binance/orders/${encodeURIComponent(trade)}/reconcile`,
        );
      if (!isApiSuccess(response.data)) throw new Error("rejected");
      await history.refresh();
    } catch {
      setError(t("Unable to update the order. Please refresh and try again."));
    } finally {
      setBusy("");
    }
  }
  return (
    <div className="space-y-6">
      <form
        id="topup-management-refresh"
        onSubmit={(e) => {
          e.preventDefault();
          void history.refresh();
        }}
      />
      {cfg && (
        <details className="rounded-2xl border bg-card p-5">
          <summary className="cursor-pointer font-semibold">
            USDT · {cfg.enabled ? t("Enabled") : t("Disabled")} · BEP20{" "}
            {cfg.networks?.BEP20 ? "✓" : "—"} · TRC20 {cfg.networks?.TRC20 ? "✓" : "—"} ·{" "}
            {t("Auto credit")} {cfg.auto_settlement_enabled ? t("Enabled") : t("Disabled")} ·{" "}
            {Number(cfg.usdt_to_vnd || 0).toLocaleString(locale)} VND · {t("Fee")}{" "}
            {cfg.fee_percent || 0}%
          </summary>
          <div className="mt-4 text-sm text-muted-foreground">
            {t("Detailed settings are managed in the current admin configuration.")}
          </div>
        </details>
      )}
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {[
          [t("Collected"), `${money.format(successfulVND)} VND`],
          [t("Today"), `${money.format(todayVND)} VND`],
          [
            t("Processing"),
            money.format(history.records.filter((r) => r.status === "pending").length),
          ],
        ].map(([label, value]) => (
          <div
            key={label}
            className="flex h-24 flex-col justify-center rounded-2xl border bg-card px-5"
          >
            <div className="text-[13px] text-muted-foreground">{label}</div>
            <div className="mt-1 text-[28px] font-bold tabular-nums">
              {query.isPending || query.isError ? "—" : value}
            </div>
          </div>
        ))}
        <div className="flex h-24 flex-col justify-center rounded-2xl border border-warning/40 bg-warning/10 px-5">
          <div className="flex items-center justify-between gap-2 text-[13px]">
            <span className="text-muted-foreground">{t("Manual review required")}</span>
            <Button
              variant="link"
              className="h-auto p-0 text-warning"
              onClick={() => setStatus("review")}
            >
              {t("View now")} →
            </Button>
          </div>
          <div className="mt-1 text-[28px] font-bold text-warning tabular-nums">
            {query.isPending || query.isError ? "—" : manual}
          </div>
        </div>
      </div>
      <Tabs value={status} onValueChange={(value) => setStatus(String(value))}>
        <section className="overflow-hidden rounded-2xl border bg-card">
          <div className="flex flex-wrap items-center justify-between gap-4 border-b px-5 py-4">
            <TabsList
              aria-label={t("Status")}
              className="h-auto! flex flex-wrap gap-1 rounded-xl border bg-background p-1"
            >
              {[
                ["all", "All transactions"],
                ["success", "Successful"],
                ["expired", "Expired"],
                ["review", "Needs review"],
              ].map(([value, label]) => (
                <TabsTrigger key={value} value={value} className="h-9 px-3">
                  {t(label)}
                  {value === "review" && <Badge variant="warning">{manual}</Badge>}
                </TabsTrigger>
              ))}
            </TabsList>
            <div className="flex flex-wrap items-center gap-2">
              <div className="relative">
                <Search
                  aria-hidden="true"
                  className="absolute left-3 top-3 size-4 text-muted-foreground"
                />
                <Input
                  className="h-10 w-64 pl-9"
                  aria-label={t("Search order ID or TxHash")}
                  placeholder={t("Search order ID or TxHash")}
                  value={q}
                  onChange={(e) => setQ(e.target.value)}
                />
              </div>
              <Select value={time} onValueChange={(v) => v && setTime(v)}>
                <SelectTrigger className="h-10" aria-label={t("Time")}>
                  <SelectValue>
                    {t("Time")}:{" "}
                    {t(
                      (
                        {
                          all: "All transactions",
                          "1": "Today",
                          "7": "Last 7 days",
                          "30": "Last 30 days",
                        } as Record<string, string>
                      )[time],
                    )}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {[
                    ["all", "All transactions"],
                    ["1", "Today"],
                    ["7", "Last 7 days"],
                    ["30", "Last 30 days"],
                  ].map(([v, label]) => (
                    <SelectItem key={v} value={v}>
                      {t(label)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Select value={method} onValueChange={(v) => v && setMethod(v)}>
                <SelectTrigger className="h-10" aria-label={t("Payment method")}>
                  <SelectValue>
                    {t("Payment method")}:{" "}
                    {method === "all"
                      ? t("All transactions")
                      : method === "binance"
                        ? "USDT"
                        : "SePay QR"}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">{t("All transactions")}</SelectItem>
                  <SelectItem value="sepay">SePay QR</SelectItem>
                  <SelectItem value="binance">USDT</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          <TabsContent value={status}>
            {(query.isError || error) && (
              <p role="alert" className="p-4 text-sm text-destructive">
                {error || t(query.error?.message ?? "Failed to load billing history")}
              </p>
            )}
            <div className="max-h-[640px] overflow-auto" aria-busy={query.isFetching}>
              <table className="w-full min-w-[1240px] table-fixed text-sm">
                <colgroup>
                  {[11, 12, 15, 14, 14, 6, 14, 14].map((width, index) => (
                    <col key={index} style={{ width: `${width}%` }} />
                  ))}
                </colgroup>
                <thead className="sticky top-0 z-10 bg-card text-left text-muted-foreground">
                  <tr className="h-11 border-b">
                    {[
                      "Time",
                      "User",
                      "Order ID",
                      "Payment method",
                      "Amount",
                      "Credit",
                      "Status",
                      "Actions",
                    ].map((h, i) => (
                      <th
                        key={h}
                        scope="col"
                        className={`px-4 text-xs font-medium ${i === 4 || i === 5 ? "text-right" : ""}`}
                      >
                        {t(h)}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {visible.map((r) => {
                    const review = needsReview(r);
                    const expired = ["Expired", "Cancelled"].includes(statusKey(r));
                    const b = r.binance;
                    const [label, tone] = state(r);
                    return (
                      <tr
                        key={r.id}
                        className={`h-14 border-b hover:bg-foreground/[0.04] ${review ? "bg-warning/[0.07]" : expired ? "text-muted-foreground" : ""}`}
                      >
                        <td
                          className={`px-4 py-2 tabular-nums ${review ? "border-l-[3px] border-l-warning" : ""}`}
                        >
                          <div>{clock.format(new Date(r.create_time * 1000))}</div>
                          <div className="text-xs text-muted-foreground">
                            {date.format(new Date(r.create_time * 1000))}
                          </div>
                        </td>
                        <td className="truncate px-4 font-medium" title={r.username}>
                          {r.username || `#${r.user_id}`}
                        </td>
                        <td className="px-4">
                          <div className="flex items-center gap-1 whitespace-nowrap font-mono text-[13px]">
                            <span title={r.trade_no}>{short(r.trade_no)}</span>
                            <Button
                              variant="ghost"
                              size="icon-sm"
                              aria-label={t("Copy order ID")}
                              onClick={() => void copyToClipboard(r.trade_no)}
                            >
                              <Copy className="size-3" />
                            </Button>
                          </div>
                        </td>
                        <td className="px-4">
                          <div>
                            {b
                              ? `USDT · ${b.network}`
                              : r.payment_provider === "sepay"
                                ? "SePay QR"
                                : r.payment_method}
                          </div>
                          {b?.deposit?.tx_id && (
                            <div
                              title={b.deposit.tx_id}
                              className="truncate font-mono text-xs text-muted-foreground"
                            >
                              {b.deposit.tx_id}
                            </div>
                          )}
                        </td>
                        <td className="px-4 text-right tabular-nums">
                          <div className="whitespace-nowrap">
                            {b
                              ? `${formatUsdt(b.amount_usdt, locale)} USDT`
                              : `${money.format(r.money)} VND`}
                          </div>
                          {b && (
                            <div className="text-xs text-muted-foreground">
                              ≈ {money.format(r.money)} VND
                            </div>
                          )}
                        </td>
                        <td className="px-4 text-right tabular-nums">
                          {r.status === "success" ? money.format(r.amount) : "—"}
                        </td>
                        <td className="px-4">
                          <Badge className={`h-auto min-h-6 border-0 px-2 py-1 ${tone}`}>
                            <span
                              aria-hidden="true"
                              className="size-1.5 shrink-0 rounded-full bg-current"
                            />
                            {label}
                          </Badge>
                        </td>
                        <td className="px-4">
                          {review && (
                            <div className="flex gap-1">
                              <Button
                                size="sm"
                                variant="outline"
                                className="h-8 px-2 text-xs"
                                disabled={busy === r.trade_no}
                                onClick={() => void action(r.trade_no, "reconcile")}
                              >
                                {t("Reconcile")}
                              </Button>
                              <Button
                                size="sm"
                                className="h-8 px-2 text-xs"
                                disabled={busy === r.trade_no}
                                onClick={() => void action(r.trade_no, "review")}
                              >
                                {t("Review")}
                              </Button>
                            </div>
                          )}
                        </td>
                      </tr>
                    );
                  })}
                  {!visible.length && (
                    <tr>
                      <td colSpan={8} className="h-28 text-center text-muted-foreground">
                        {query.isPending
                          ? t("Loading...")
                          : query.isError
                            ? t("Failed to load billing history")
                            : t("No transactions found")}
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
            <div className="flex flex-wrap items-center justify-between gap-4 px-5 py-4 text-sm">
              <span className="text-muted-foreground">
                {t("Showing {{from}}–{{to}} / {{total}} transactions", {
                  from: rows.length ? (current - 1) * pageSize + 1 : 0,
                  to: Math.min(current * pageSize, rows.length),
                  total: rows.length,
                })}
              </span>
              <div className="flex items-center gap-2">
                <Select
                  value={String(pageSize)}
                  onValueChange={(v) => {
                    if (v) setPageSize(Number(v));
                  }}
                >
                  <SelectTrigger aria-label={t("Rows per page")}>
                    <SelectValue>
                      {pageSize} / {t("page")}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {[10, 20, 50, 100].map((size) => (
                      <SelectItem key={size} value={String(size)}>
                        {size} / {t("page")}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <nav aria-label={t("Pagination")} className="flex items-center gap-1">
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label={t("Previous page")}
                    disabled={current <= 1}
                    onClick={() => setPage(current - 1)}
                  >
                    <ChevronLeft className="size-4" />
                  </Button>
                  {Array.from(
                    { length: Math.min(5, pages) },
                    (_, i) => Math.max(1, Math.min(current - 2, pages - 4)) + i,
                  ).map((n) => (
                    <Button
                      key={n}
                      size="icon-sm"
                      variant={n === current ? "default" : "ghost"}
                      aria-label={t("Page {{number}}", { number: n })}
                      aria-current={n === current ? "page" : undefined}
                      onClick={() => setPage(n)}
                    >
                      {n}
                    </Button>
                  ))}
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label={t("Next page")}
                    disabled={current >= pages}
                    onClick={() => setPage(current + 1)}
                  >
                    <ChevronRight className="size-4" />
                  </Button>
                </nav>
              </div>
            </div>
          </TabsContent>
        </section>
      </Tabs>
    </div>
  );
}
