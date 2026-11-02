import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { TopupRecord } from "@/features/wallet/types";
import {
  TopupManagementTable,
  formatUsdt,
  loadTopups,
  needsReview,
  statusKey,
} from "./topup-management-table";

const mocks = vi.hoisted(() => ({ getAll: vi.fn(), get: vi.fn(), post: vi.fn(), copy: vi.fn() }));
vi.mock("@/features/wallet/api", () => ({
  getAllBillingHistory: mocks.getAll,
  isApiSuccess: (r: { success: boolean }) => r.success,
}));
vi.mock("@/lib/api", () => ({ api: { get: mocks.get, post: mocks.post } }));
vi.mock("@/hooks/use-copy-to-clipboard", () => ({
  useCopyToClipboard: () => ({ copyToClipboard: mocks.copy }),
}));
vi.mock("@/hooks/use-media-query", () => ({ useMediaQuery: () => false }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, values?: Record<string, unknown>) =>
      key.replace(/{{(\w+)}}/g, (_, name) => String(values?.[name] ?? name)),
    i18n: { language: "en" },
  }),
}));

const record = (id: number, overrides: Partial<TopupRecord> = {}): TopupRecord => ({
  id,
  user_id: id,
  username: `customer-${id}`,
  amount: 260,
  money: 260000,
  trade_no: `ORDER-${id}`,
  payment_method: "sepay_bank_transfer",
  payment_provider: "sepay",
  status: "success",
  create_time: 1700000000 + id,
  ...overrides,
});
const expiredBinance = record(99, {
  create_time: 1699999999,
  status: "expired",
  binance: {
    status: "expired",
    amount_usdt: "10.200001",
    network: "BEP20",
    deposit: { tx_id: "tx-unique" },
  },
});
afterEach(() => {
  cleanup();
  vi.resetAllMocks();
});

describe("top-up management presentation", () => {
  it("keeps cancelled/expired bank orders distinct from waiting and Binance review", () => {
    expect(statusKey(record(1, { status: "expired" }))).toBe("Expired");
    expect(needsReview(record(1, { status: "expired" }))).toBe(false);
    expect(
      statusKey(
        record(2, {
          status: "expired",
          sepay: {
            credits: 0,
            amount_vnd: 10000,
            verification_status: "voided",
            verification_reason: "cancelled by user",
          },
        }),
      ),
    ).toBe("Cancelled");
    expect(needsReview(expiredBinance)).toBe(true);
    expect(needsReview({ ...expiredBinance, status: "success" })).toBe(false);
    expect(formatUsdt("10.200000", "vi-VN")).toBe("10,2");
    expect(formatUsdt("10.200001", "en-US")).toBe("10.200001");
    expect(formatUsdt("100", "en-US")).toBe("100");
  });

  it("reads every API page and fails explicitly on errors or oversized history", async () => {
    mocks.getAll
      .mockResolvedValueOnce({
        success: true,
        data: { items: Array.from({ length: 100 }, (_, i) => record(i)), total: 101 },
      })
      .mockResolvedValueOnce({ success: true, data: { items: [record(100)], total: 101 } });
    expect(await loadTopups()).toHaveLength(101);
    expect(mocks.getAll).toHaveBeenLastCalledWith(2, 100);
    mocks.getAll.mockResolvedValueOnce({ success: false });
    await expect(loadTopups()).rejects.toThrow("Failed to load");
    mocks.getAll.mockResolvedValueOnce({ success: true, data: { items: [], total: 10001 } });
    await expect(loadTopups()).rejects.toThrow("display limit");
  });

  it("does not publish incomplete totals or continue loading after cancellation", async () => {
    mocks.getAll.mockResolvedValueOnce({ success: true, data: { items: [], total: 2 } });
    await expect(loadTopups()).rejects.toThrow("History changed");
    const controller = new AbortController();
    controller.abort();
    mocks.getAll.mockClear();
    await expect(loadTopups(controller.signal)).rejects.toThrow();
    expect(mocks.getAll).not.toHaveBeenCalled();
  });

  it("keeps chronological order without pinning review rows and supports review filters", async () => {
    const items = [...Array.from({ length: 11 }, (_, i) => record(i + 1)), expiredBinance];
    mocks.getAll.mockResolvedValue({ success: true, data: { items, total: items.length } });
    mocks.get.mockResolvedValue({
      data: {
        success: true,
        data: {
          enabled: true,
          networks: { BEP20: true, TRC20: true },
          usdt_to_vnd: "26000",
          fee_percent: "2",
        },
      },
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <TopupManagementTable />
      </QueryClientProvider>,
    );
    await screen.findByText("customer-11");
    const table = screen.getByRole("table");
    expect(within(table).getAllByRole("row")).toHaveLength(11);
    expect(within(table).getAllByRole("row")[1].textContent).toContain("customer-11");
    expect(within(table).queryByText("customer-99")).toBeNull();
    expect(within(table).queryByRole("columnheader", { name: "TxHash" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Next page" }));
    expect(within(table).getAllByRole("row")).toHaveLength(3);
    expect(within(table).getAllByRole("row")[2].textContent).toContain("customer-99");
    fireEvent.change(screen.getByRole("textbox", { name: "Search order ID or TxHash" }), {
      target: { value: "tx-unique" },
    });
    expect(within(table).getAllByRole("row")).toHaveLength(2);
    fireEvent.change(screen.getByRole("textbox", { name: "Search order ID or TxHash" }), {
      target: { value: "" },
    });
    fireEvent.click(screen.getByRole("button", { name: "View now →" }));
    expect(within(table).getAllByRole("row")).toHaveLength(2);
    fireEvent.click(screen.getByRole("button", { name: "Copy order ID" }));
    expect(mocks.copy).toHaveBeenCalledWith("ORDER-99");
    mocks.post.mockResolvedValueOnce({ data: { success: false } });
    fireEvent.click(screen.getByRole("button", { name: "Reconcile" }));
    await screen.findByRole("alert");
    expect(mocks.post).toHaveBeenCalledWith("/api/user/binance/orders/ORDER-99/reconcile");
    const beforeRefresh = mocks.getAll.mock.calls.length;
    fireEvent.submit(document.getElementById("topup-management-refresh")!);
    await waitFor(() => expect(mocks.getAll.mock.calls.length).toBeGreaterThan(beforeRefresh));
    client.clear();
  });
});
