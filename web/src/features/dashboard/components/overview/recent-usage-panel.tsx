import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { z } from "zod";
import { RefreshCw } from "lucide-react";

import { getAllLogs, getUserLogs } from "@/features/usage-logs/api";
import { usageLogSchema } from "@/features/usage-logs/data/schema";
import { getSelf, getUserModels } from "@/lib/api";
import { formatQuotaWithCredit } from "@/lib/currency";
import { formatTimestampToDate } from "@/lib/format";
import { ROLE } from "@/lib/roles";
import { requireServerSuccess } from "@/lib/server-error-message";
import { cn } from "@/lib/utils";
import { useAuthStore } from "@/stores/auth-store";

import type { UnixRange } from "./overview-time-range";

const balanceSchema = z.object({
  id: z.number(),
  quota: z.number().finite(),
  used_quota: z.number().finite(),
  request_count: z.number().finite(),
});

function formatLogTime(createdAt: number): string {
  if (createdAt > 1_000_000_000) {
    return new Date(createdAt * 1000).toLocaleTimeString("vi-VN", {
      hour12: false,
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });
  }
  return formatTimestampToDate(createdAt);
}

// Foreground polling shares the server's settled balance; never deduct locally.
export function useDashboardBalance(enabled = true) {
  const userId = useAuthStore((state) => state.auth.user?.id);
  const sessionId = useAuthStore((state) => state.auth.session?.sid);
  const query = useQuery({
    queryKey: ["dashboard", "settled-balance", userId, sessionId],
    enabled: enabled && Boolean(userId),
    queryFn: async () => {
      const self = await getSelf();
      requireServerSuccess(self);
      if (!self.data || self.data.id !== userId) {
        throw new Error("Invalid balance response");
      }
      return balanceSchema.parse(self.data);
    },
    refetchInterval: 15000,
    refetchIntervalInBackground: false,
    retry: false,
  });

  useEffect(() => {
    if (!enabled || !query.data) return;
    const auth = useAuthStore.getState().auth;
    if (!auth.user || auth.user.id !== userId || auth.session?.sid !== sessionId) return;
    auth.setUser({ ...auth.user, ...query.data });
  }, [enabled, query.data, userId, sessionId]);
  return query;
}

// Overview rows show at most 4 decimals; the full logs page keeps 6.
function formatRowQuota(quota: number): string {
  return formatQuotaWithCredit(quota, { abbreviate: false, digitsLarge: 2, digitsSmall: 4 });
}

const DEFAULT_VISIBLE_ROWS = 6;

// range limits the list and its total to the page's period (admin Overview,
// titled "Recent requests"); without it the panel shows the latest requests of
// all time. limit caps the visible rows.
// excludeInternal hides admin and root accounts' rows (admin endpoint only).
export function RecentUsagePanel(
  props: { range?: UnixRange | null; limit?: number; excludeInternal?: boolean } = {},
) {
  const { t } = useTranslation();
  const userId = useAuthStore((state) => state.auth.user?.id);
  const sessionId = useAuthStore((state) => state.auth.session?.sid);
  const isAdmin = useAuthStore((state) => state.auth.user?.role === ROLE.SUPER_ADMIN);
  const [refreshInterval, setRefreshInterval] = useState(30000);
  const [usernameFilter, setUsernameFilter] = useState("");
  const [modelFilter, setModelFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState<"all" | "success" | "error">("all");
  const useAllEndpoint = isAdmin;
  const balanceQuery = useDashboardBalance();
  const modelsQuery = useQuery({
    queryKey: ["dashboard", "usage-models", userId, sessionId],
    enabled: Boolean(userId),
    queryFn: async () => {
      const result = await getUserModels();
      requireServerSuccess(result);
      return result.data ?? [];
    },
    staleTime: 5 * 60 * 1000,
    retry: false,
  });
  const query = useQuery({
    queryKey: [
      "dashboard",
      "recent-usage",
      userId,
      sessionId,
      isAdmin,
      useAllEndpoint,
      usernameFilter,
      modelFilter,
      statusFilter,
      props.range?.start_timestamp,
      props.range?.end_timestamp,
      props.excludeInternal,
    ],
    enabled: Boolean(userId),
    queryFn: async () => {
      const params = {
        page_size: 10,
        p: 1,
        ...(props.range
          ? {
              start_timestamp: props.range.start_timestamp,
              end_timestamp: props.range.end_timestamp,
            }
          : {}),
        ...(usernameFilter.trim() && useAllEndpoint ? { username: usernameFilter.trim() } : {}),
        ...(props.excludeInternal && useAllEndpoint ? { exclude_internal: true } : {}),
        ...(modelFilter.trim() ? { model_name: modelFilter.trim() } : {}),
        ...(statusFilter === "error" ? { type: 5 } : statusFilter === "success" ? { type: 2 } : {}),
      };
      const logs = await (useAllEndpoint ? getAllLogs : getUserLogs)(params);
      requireServerSuccess(logs);
      if (!logs.data) throw new Error("Invalid usage response");
      const parsedLogs = usageLogSchema.array().parse(logs.data.items);
      return {
        logs: parsedLogs,
        total: typeof logs.data.total === "number" ? logs.data.total : parsedLogs.length,
      };
    },
    refetchInterval: refreshInterval,
    refetchIntervalInBackground: false,
    retry: false,
  });

  const visibleLogs = (query.data?.logs ?? []).slice(0, props.limit ?? DEFAULT_VISIBLE_ROWS);
  const totalCount = query.data?.total ?? visibleLogs.length;
  const modelOptions = Array.from(
    new Set([...(modelsQuery.data ?? []), ...(modelFilter ? [modelFilter] : [])]),
  ).sort((a, b) => a.localeCompare(b));

  const handleRefresh = () => {
    void Promise.all([query.refetch(), balanceQuery.refetch()]);
  };

  return (
    <section className="bg-card min-w-0 space-y-4 rounded-2xl border p-4 sm:p-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">
            {props.range ? t("Recent requests") : t("Usage logs")}
          </h2>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <label className="text-muted-foreground inline-flex items-center gap-1.5 text-xs">
            <span>{t("Refresh")}</span>
            <select
              aria-label={t("Refresh interval")}
              value={refreshInterval}
              onChange={(event) => setRefreshInterval(Number(event.target.value))}
              className="bg-background border-border h-9 rounded-xl border px-2 text-xs text-foreground"
            >
              <option value={30000}>{t("Every 30 seconds")}</option>
              <option value={300000}>{t("Every 5 minutes")}</option>
              <option value={900000}>{t("Every 15 minutes")}</option>
            </select>
          </label>
          <button
            type="button"
            onClick={handleRefresh}
            disabled={query.isFetching || balanceQuery.isFetching}
            className="border-border bg-background/40 hover:bg-accent inline-flex h-9 items-center gap-1.5 rounded-xl border px-3 text-xs font-medium transition-colors"
          >
            <RefreshCw className={cn("size-3.5", query.isFetching && "animate-spin")} />
            {t("Load new logs")}
          </button>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        {isAdmin && useAllEndpoint && (
          <input
            value={usernameFilter}
            onChange={(event) => setUsernameFilter(event.target.value)}
            placeholder={t("Username")}
            aria-label={t("Username")}
            className="bg-background border-border h-9 w-36 rounded-xl border px-3 text-xs outline-none"
          />
        )}
        <select
          value={modelFilter}
          onChange={(event) => setModelFilter(event.target.value)}
          aria-label={t("Model")}
          className="bg-background border-border h-9 min-w-40 rounded-xl border px-3 text-xs text-foreground outline-none"
        >
          <option value="">{t("All Models")}</option>
          {modelOptions.map((model) => (
            <option key={model} value={model}>
              {model}
            </option>
          ))}
        </select>
        <select
          value={statusFilter}
          onChange={(event) => setStatusFilter(event.target.value as typeof statusFilter)}
          aria-label={t("Request status")}
          className="bg-background border-border h-9 rounded-xl border px-3 text-xs text-foreground"
        >
          <option value="all">{t("All statuses")}</option>
          <option value="success">{t("Success")}</option>
          <option value="error">{t("Error")}</option>
        </select>
      </div>

      {(query.isError || balanceQuery.isError) && (
        <p role="alert" className="text-destructive text-sm">
          {t("Could not refresh usage and balance. Displayed data may be outdated.")}
        </p>
      )}

      {query.isPending ? (
        <p className="text-muted-foreground py-4 text-sm">{t("Loading")}</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs sm:text-sm">
            <thead className="text-muted-foreground text-xs">
              <tr>
                <th className="px-2.5 py-2 font-medium">{t("Time")}</th>
                {isAdmin && <th className="px-2.5 py-2 font-medium">{t("User")}</th>}
                <th className="px-2.5 py-2 font-medium">{t("Model")}</th>
                <th className="px-2.5 py-2 text-right font-medium">{t("Cost")}</th>
                <th className="px-2.5 py-2 font-medium">{t("Status")}</th>
              </tr>
            </thead>
            <tbody>
              {visibleLogs.filter(Boolean).map((log, index) => {
                const isError = log.type === 5;

                return (
                  <tr
                    key={log.id || index}
                    className={cn(
                      "border-border/60 border-t transition-colors",
                      isError && "bg-red-500/10 text-red-400",
                    )}
                  >
                    <td className="px-2.5 py-3 font-mono text-xs whitespace-nowrap tabular-nums">
                      {formatLogTime(log.created_at)}
                    </td>
                    {isAdmin && (
                      <td className="px-2.5 py-3 whitespace-nowrap">{log.username || "—"}</td>
                    )}
                    <td className="px-2.5 py-3 font-mono text-xs whitespace-nowrap">
                      {log.model_name || "—"}
                    </td>
                    <td className="px-2.5 py-3 text-right font-mono text-xs whitespace-nowrap tabular-nums">
                      {formatRowQuota(log.quota)}
                    </td>
                    <td className="px-2.5 py-3 whitespace-nowrap">
                      <span
                        className={cn(
                          "inline-flex items-center rounded-full px-2.5 py-0.5 text-[11px] font-medium",
                          isError
                            ? "bg-red-500/15 text-red-400"
                            : "bg-emerald-500/15 text-emerald-400",
                        )}
                      >
                        {t(isError ? "Error" : log.type === 2 ? "Consume" : "Other")}
                      </span>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
          {visibleLogs.length === 0 && (
            <p className="text-muted-foreground p-3 text-sm">{t("No data")}</p>
          )}
        </div>
      )}

      <div className="flex flex-wrap items-center justify-between gap-2 pt-1">
        <p className="text-muted-foreground text-xs">
          {t("Showing {{shown}} of {{total}} requests", {
            shown: visibleLogs.length,
            total: new Intl.NumberFormat().format(totalCount),
          })}
        </p>
        <Link
          to="/dashboard/$section"
          params={{ section: "logs" }}
          className="border-border bg-background/40 hover:bg-accent inline-flex h-9 items-center justify-center rounded-xl border px-4 text-xs font-medium transition-colors"
        >
          {t("View all logs")}
        </Link>
      </div>
    </section>
  );
}
