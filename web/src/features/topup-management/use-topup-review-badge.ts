import { useQuery } from "@tanstack/react-query";
import { getAdminTopupList, isApiSuccess } from "@/features/wallet/api";

export const TOPUP_REVIEW_BADGE_REFRESH_MS = 60_000;

/** Reads the needs-review count from the admin list summary (one row requested). */
export async function loadManualReviewCount(signal?: AbortSignal): Promise<number> {
  signal?.throwIfAborted();
  const response = await getAdminTopupList(
    { page: 1, pageSize: 1, status: "needs_review" },
    // Silent: a failed badge poll must not raise a toast.
    { signal, skipErrorHandler: true },
  );
  if (!isApiSuccess(response) || !response.data)
    throw new Error("Failed to load top-up review count");
  return response.data.summary?.needs_review ?? 0;
}

/** Sidebar badge text for the manual-review count, or undefined when hidden. */
export function useTopupReviewBadge(enabled: boolean): string | undefined {
  const query = useQuery({
    queryKey: ["admin-topup-review-badge"],
    queryFn: ({ signal }) => loadManualReviewCount(signal),
    enabled,
    refetchInterval: TOPUP_REVIEW_BADGE_REFRESH_MS,
    refetchOnWindowFocus: false,
    retry: false,
  });
  return query.data && query.data > 0 ? String(query.data) : undefined;
}
