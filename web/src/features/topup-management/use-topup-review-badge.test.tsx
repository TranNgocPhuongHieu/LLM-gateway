import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider, type QueryClientConfig } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { TOPUP_REVIEW_BADGE_REFRESH_MS, useTopupReviewBadge } from "./use-topup-review-badge";

const mocks = vi.hoisted(() => ({ list: vi.fn() }));
vi.mock("@/features/wallet/api", () => ({
  getAdminTopupList: mocks.list,
  isApiSuccess: (r: { success: boolean }) => r.success,
}));

const wrapper =
  (options: QueryClientConfig = {}) =>
  ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={new QueryClient(options)}>{children}</QueryClientProvider>
  );

afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

describe("useTopupReviewBadge", () => {
  it("reads the needs-review count with one row and a silent error handler", async () => {
    mocks.list.mockResolvedValue({ success: true, data: { items: [], total: 3, summary: { needs_review: 3 } } });
    const { result } = renderHook(() => useTopupReviewBadge(true), { wrapper: wrapper() });
    await waitFor(() => expect(result.current).toBe("3"));
    expect(mocks.list).toHaveBeenCalledWith(
      { page: 1, pageSize: 1, status: "needs_review" },
      expect.objectContaining({ skipErrorHandler: true }),
    );
    expect(TOPUP_REVIEW_BADGE_REFRESH_MS).toBe(60_000);
  });

  it("is hidden when the count is zero", async () => {
    mocks.list.mockResolvedValue({ success: true, data: { items: [], total: 0, summary: { needs_review: 0 } } });
    const { result } = renderHook(() => useTopupReviewBadge(true), { wrapper: wrapper() });
    await waitFor(() => expect(mocks.list).toHaveBeenCalled());
    await waitFor(() => expect(result.current).toBeUndefined());
  });

  it("stays hidden and silent when the request fails", async () => {
    mocks.list.mockResolvedValue({ success: false });
    const { result } = renderHook(() => useTopupReviewBadge(true), { wrapper: wrapper() });
    await waitFor(() => expect(mocks.list).toHaveBeenCalled());
    expect(result.current).toBeUndefined();
  });

  it("does not request anything for users who are not super admins", () => {
    renderHook(() => useTopupReviewBadge(false), { wrapper: wrapper() });
    expect(mocks.list).not.toHaveBeenCalled();
  });
});
