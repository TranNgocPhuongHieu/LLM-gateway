import { api } from "@/lib/http-client";

export type TokenUsage = {
  name: string;
  total_granted: number;
  total_used: number;
  total_available: number;
  unlimited_quota: boolean;
  model_limits?: Record<string, boolean>;
  model_limits_enabled?: boolean;
  expires_at: number;
  quota_mode?: "billing" | "token";
};

export type UsageLog = {
  id: number;
  created_at: number;
  type: number;
  model_name: string;
  quota: number;
  prompt_tokens: number;
  completion_tokens: number;
  use_time: number;
  is_stream: boolean;
  request_id?: string;
  content?: string;
  raw_token_usage?: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  input_tokens_total: number;
};

export async function getPublicQuota(key: string) {
  const headers = { Authorization: `Bearer ${key}` };
  const usageResponse = await api
    .get("/api/usage/token/", {
      headers,
      skipSessionAuth: true,
      skipAuthRefresh: true,
      skipErrorHandler: true,
      disableDuplicate: true,
    })
    .catch((error) => {
      const status = (error as { response?: { status?: number } })?.response?.status;
      throw new Error(
        status === 401 ? "API key không hợp lệ hoặc đã bị khóa." : "Không thể đọc quota API key.",
      );
    });

  const usage = usageResponse.data?.data as TokenUsage | undefined;
  if (!usage) {
    throw new Error(
      usageResponse.status === 401
        ? "API key không hợp lệ hoặc đã bị khóa."
        : "Không thể đọc quota API key.",
    );
  }
  let logs: UsageLog[] = [];
  let logsWarning: string | undefined;
  try {
    const logsResponse = await api.get("/api/log/token", {
      headers,
      skipSessionAuth: true,
      skipAuthRefresh: true,
      skipErrorHandler: true,
      disableDuplicate: true,
    });
    const rawLogs = logsResponse.data?.data;
    if (rawLogs != null && !Array.isArray(rawLogs)) {
      throw new Error("Invalid log response");
    }
    logs = (rawLogs ?? []).map((row: UsageLog & { other?: string }) => {
      const metadata = row.other ? JSON.parse(row.other) : {};
      return {
        ...row,
        cache_read_tokens:
          typeof metadata.cache_tokens === "number" ? metadata.cache_tokens : 0,
        cache_write_tokens:
          typeof metadata.cache_write_tokens === "number"
            ? metadata.cache_write_tokens
            : typeof metadata.cache_creation_tokens === "number"
              ? metadata.cache_creation_tokens
              : 0,
        input_tokens_total:
          typeof metadata.input_tokens_total === "number"
            ? metadata.input_tokens_total
            : String(metadata.usage_semantic || "").toLowerCase() === "anthropic"
              ? (Number(row.prompt_tokens) || 0) +
                (typeof metadata.cache_tokens === "number" ? metadata.cache_tokens : 0) +
                (typeof metadata.cache_write_tokens === "number"
                  ? metadata.cache_write_tokens
                  : typeof metadata.cache_creation_tokens === "number"
                    ? metadata.cache_creation_tokens
                    : 0)
              : Number(row.prompt_tokens) || 0,
        raw_token_usage:
          typeof metadata.raw_token_usage === "number" ? metadata.raw_token_usage : undefined,
      };
    });
  } catch {
    logsWarning =
      "Không đọc được lịch sử request. Số dư của key vẫn được hiển thị; không thể kết luận key chưa có hoạt động.";
  }
  return { usage, logs, logsWarning };
}
