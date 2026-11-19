import { ChevronLeft, ChevronRight, Copy, Search } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useEffect, useMemo, useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
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
import {
  getAdminTopupList,
  isApiSuccess,
  type AdminTopupListResponse,
} from "@/features/wallet/api";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { useDebounce } from "@/hooks/use-debounce";
import type { TopupRecord } from "@/features/wallet/types";
import { api } from "@/lib/api";

const short = (v: string) => (v.length > 12 ? `${v.slice(0, 7)}…${v.slice(-4)}` : v);
export function formatUsdt(value: string, locale: string) {
  const compact = value.includes(".") ? value.replace(/0+$/, "").replace(/\.$/, "") : value;
  return locale.startsWith("vi") ? compact.replace(".", ",") : compact;
}
export const needsReview = (r: TopupRecord) =>
  r.status !== "success" &&
  (["review", "partial", "matched", "expired"].includes(r.binance?.status ?? "") ||
    r.sepay?.verification_status === "review");

export const canConfirmBank = (r: TopupRecord) =>
  r.payment_provider === "sepay" && r.sepay?.flow === "bank" &&
  ((r.status === "pending" && ["pending", "review"].includes(r.sepay.verification_status)) ||
    (r.status === "expired" && r.sepay.verification_status === "voided" &&
      r.sepay.verification_reason === "QR payment expired after 15 minutes"));

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

export type TopupListQuery = {
  page: number;
  pageSize: number;
  keyword: string;
  status: string;
  paymentMethod: string;
  time: string;
};

/** Local-day bounds in unix seconds; the end is exclusive. */
export function localDayBounds(now = new Date()) {
  const start = new Date(now);
  start.setHours(0, 0, 0, 0);
  const end = new Date(start);
  end.setDate(end.getDate() + 1);
  return {
    start: Math.floor(start.getTime() / 1000),
    end: Math.floor(end.getTime() / 1000),
  };
}

// UI tab value -> server status filter ("all" is absent and means no filter).
const TOPUP_STATUS_PARAM: Record<string, string | undefined> = {
  success: "success",
  expired: "expired",
  review: "needs_review",
};
const TOPUP_TIME_DAYS: Record<string, number> = { "7": 7, "30": 30 };

/** Loads one server-side page of the admin list together with its summary. */
export async function loadTopupPage(
  query: TopupListQuery,
  signal?: AbortSignal,
): Promise<AdminTopupListResponse> {
  signal?.throwIfAborted();
  const today = localDayBounds();
  let startTimestamp: number | undefined;
  if (query.time === "1") startTimestamp = today.start;
  else if (query.time in TOPUP_TIME_DAYS)
    startTimestamp = Math.floor(Date.now() / 1000) - TOPUP_TIME_DAYS[query.time] * 86400;
  const response = await getAdminTopupList(
    {
      page: query.page,
      pageSize: query.pageSize,
      keyword: query.keyword,
      status: TOPUP_STATUS_PARAM[query.status],
      paymentMethod: query.paymentMethod === "all" ? undefined : query.paymentMethod,
      startTimestamp,
      todayStart: today.start,
      todayEnd: today.end,
    },
    { signal },
  );
  if (!isApiSuccess(response) || !response.data)
    throw new Error("Failed to load billing history");
  return response.data;
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
  const keyword = useDebounce(q.trim(), 300);
  const [method, setMethod] = useState("all");
  const [status, setStatus] = useState("all");
  const [time, setTime] = useState("all");
  const [busy, setBusy] = useState("");
  const listQuery = useQuery({
    queryKey: ["admin-topup-list", keyword, page, pageSize, status, method, time],
    queryFn: ({ signal }) =>
      loadTopupPage({ page, pageSize, keyword, status, paymentMethod: method, time }, signal),
    placeholderData: keepPreviousData,
    retry: false,
    refetchOnWindowFocus: false,
  });
  const summary = listQuery.data?.summary;
  const history = { refresh: listQuery.refetch };
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
  // Status, payment method and time are filtered by the server.
  const rows = useMemo(
    () => [...(listQuery.data?.items ?? [])].sort((a, b) => b.create_time - a.create_time),
    [listQuery.data],
  );
  const processing = summary?.processing ?? 0;
  const manual = summary?.needs_review ?? 0;
  const successfulVND = summary?.collected_vnd ?? 0;
  const todayVND = summary?.today_vnd ?? 0;
  // Pages come from the server total; `page` is the requested server page.
  const total = listQuery.data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / pageSize));
  const current = page;
  const loadError = listQuery.error;
  async function confirmBank(r: TopupRecord) {
    if (!window.confirm(t("Confirm receipt for {{user}}, order {{order}}: {{money}} VND → {{credits}} credits. Previously added credits will NOT be deducted. Continue?", {
      user: r.username ?? r.user_id, order: r.trade_no,
      money: money.format(r.sepay!.amount_vnd), credits: money.format(r.sepay!.credits),
    }))) return;
    setBusy(r.trade_no);
    setError("");
    try {
      const response = await api.post(`/api/user/sepay/orders/${encodeURIComponent(r.trade_no)}/confirm`, {
        received: true,
      });
      if (!isApiSuccess(response.data)) throw new Error("rejected");
      await history.refresh();
    } catch {
      setError(t("Unable to update the order. Please refresh and try again."));
    } finally {
      setBusy("");
    }
  }
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
            money.format(processing),
          ],
        ].map(([label, value]) => (
          <div
            key={label}
            className="flex h-24 flex-col justify-center rounded-2xl border bg-card px-5"
          >
            <div className="text-[13px] text-muted-foreground">{label}</div>
            <div className="mt-1 text-[28px] font-bold tabular-nums">
              {summary ? value : "—"}
            </div>
          </div>
        ))}
        <div className="flex h-24 flex-col justify-center rounded-2xl border border-warning/40 bg-warning/10 px-5">
          <div className="flex items-center justify-between gap-2 text-[13px]">
            <span className="text-muted-foreground">{t("Manual review required")}</span>
            <Button
              variant="link"
              className="h-auto p-0 text-warning"
              onClick={() => {
                setStatus("review");
                setPage(1);
              }}
            >
              {t("View now")} →
            </Button>
          </div>
          <div className="mt-1 text-[28px] font-bold text-warning tabular-nums">
            {summary ? manual : "—"}
          </div>
        </div>
      </div>
      <Tabs
        value={status}
        onValueChange={(value) => {
          setStatus(String(value));
          setPage(1);
        }}
      >
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
                  onChange={(e) => {
                    setQ(e.target.value);
                    setPage(1);
                  }}
                />
              </div>
              <Select
                value={time}
                onValueChange={(v) => {
                  if (v) {
                    setTime(v);
                    setPage(1);
                  }
                }}
              >
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
              <Select
                value={method}
                onValueChange={(v) => {
                  if (v) {
                    setMethod(v);
                    setPage(1);
                  }
                }}
              >
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
            {(listQuery.isError || error) && (
              <p role="alert" className="p-4 text-sm text-destructive">
                {error || t(loadError?.message ?? "Failed to load billing history")}
              </p>
            )}
            <div className="max-h-[640px] overflow-auto" aria-busy={listQuery.isFetching}>
              <table className="w-full min-w-[1040px] table-fixed text-sm">
                <colgroup>
                  {[9, 11, 14, 13, 13, 7, 15, 18].map((width, index) => (
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
                        className={`px-3 text-xs font-medium ${i === 4 || i === 5 ? "text-right" : ""}`}
                      >
                        {t(h)}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {rows.map((r) => {
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
                          className={`px-3 py-2 tabular-nums ${review ? "border-l-[3px] border-l-warning" : ""}`}
                        >
                          <div>{clock.format(new Date(r.create_time * 1000))}</div>
                          <div className="text-xs text-muted-foreground">
                            {date.format(new Date(r.create_time * 1000))}
                          </div>
                        </td>
                        <td className="truncate px-3 font-medium" title={r.username}>
                          {r.username || `#${r.user_id}`}
                        </td>
                        <td className="px-3">
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
                        <td className="px-3">
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
                        <td className="px-3 text-right tabular-nums">
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
                        <td className="px-3 text-right tabular-nums">
                          {r.status === "success" ? money.format(r.amount) : "—"}
                        </td>
                        <td className="px-3">
                          <Badge className={`h-auto min-h-6 border-0 px-2 py-1 ${tone}`}>
                            <span
                              aria-hidden="true"
                              className="size-1.5 shrink-0 rounded-full bg-current"
                            />
                            {label}
                          </Badge>
                        </td>
                        <td className="px-3">
                          {canConfirmBank(r) && (
                            <Button
                              size="sm"
                              className="h-8 whitespace-nowrap px-2 text-xs"
                              disabled={busy !== ""}
                              onClick={() => void confirmBank(r)}
                            >
                              {t("Confirm bank receipt")}
                            </Button>
                          )}
                          {/* Reconcile/Review call the Binance order endpoints; SePay
                              review rows are resolved with Confirm bank receipt. */}
                          {review && b && (
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
                  {!rows.length && (
                    <tr>
                      <td colSpan={8} className="h-28 text-center text-muted-foreground">
                        {listQuery.isPending
                          ? t("Loading...")
                          : listQuery.isError
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
                  from: total ? (current - 1) * pageSize + 1 : 0,
                  to: Math.min(current * pageSize, total),
                  total,
                })}
              </span>
              <div className="flex items-center gap-2">
                <Select
                  value={String(pageSize)}
                  onValueChange={(v) => {
                    if (v) {
                      setPageSize(Number(v));
                      setPage(1);
                    }
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
