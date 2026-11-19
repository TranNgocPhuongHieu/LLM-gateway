import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { TopupRecord } from "@/features/wallet/types";
import {
  TopupManagementTable,
  formatUsdt,
  loadTopupPage,
  localDayBounds,
  needsReview,
  canConfirmBank,
  statusKey,
} from "./topup-management-table";

const mocks = vi.hoisted(() => ({ getAll: vi.fn(), get: vi.fn(), post: vi.fn(), copy: vi.fn() }));
vi.mock("@/features/wallet/api", () => ({
  getAdminTopupList: mocks.getAll,
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
type FakeParams = {
  page: number;
  pageSize: number;
  keyword?: string;
  status?: string;
  paymentMethod?: string;
  startTimestamp?: number;
};
// Fake list endpoint: applies the same filters the server applies (keyword matches
// trade_no only here), newest first, then the requested page.
const serve =
  (items: TopupRecord[], summary = {}) =>
  (params: FakeParams) => {
    const keyword = params.keyword ?? "";
    const matched = items
      .filter((r) => !keyword || r.trade_no.toLowerCase().includes(keyword.toLowerCase()))
      .filter(
        (r) =>
          !params.status ||
          (params.status === "success" && r.status === "success") ||
          (params.status === "expired" && r.status === "expired") ||
          (params.status === "needs_review" && needsReview(r)),
      )
      .filter(
        (r) =>
          !params.paymentMethod ||
          (params.paymentMethod === "binance" ? !!r.binance : r.payment_provider === "sepay"),
      )
      .filter((r) => !params.startTimestamp || r.create_time >= params.startTimestamp)
      .sort((a, b) => b.create_time - a.create_time);
    const page = params.page;
    return {
      success: true,
      data: {
        items: matched.slice((page - 1) * params.pageSize, page * params.pageSize),
        total: matched.length,
        summary,
      },
    };
  };
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
const zeroSummary = {
  total: 0,
  pending: 0,
  success: 0,
  cancelled: 0,
  processing: 0,
  needs_review: 0,
  collected_vnd: 0,
  today_vnd: 0,
};
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.resetAllMocks();
});

describe("top-up management presentation", () => {
  it("confirms bank receipt with a single acknowledgement and no transaction ID", async () => {
    const bank = record(1, { status: "pending", sepay: { flow: "bank", credits: 600, amount_vnd: 500000, verification_status: "pending" } });
    mocks.getAll.mockResolvedValue({ success: true, data: { items: [bank], total: 1, summary: zeroSummary } });
    mocks.get.mockResolvedValue({ data: { success: false } });
    mocks.post.mockResolvedValue({ data: { success: true } });
    const prompt = vi.spyOn(window, "prompt");
    const confirm = vi.spyOn(window, "confirm").mockReturnValueOnce(false).mockReturnValueOnce(true);
    render(<QueryClientProvider client={new QueryClient()}><TopupManagementTable /></QueryClientProvider>);
    const button = await screen.findByRole("button", { name: "Confirm bank receipt" });
    fireEvent.click(button);
    expect(confirm).toHaveBeenCalledWith(expect.stringContaining("600 credits"));
    expect(mocks.post).not.toHaveBeenCalled();
    fireEvent.click(button);
    await waitFor(() => expect(mocks.post).toHaveBeenCalledWith("/api/user/sepay/orders/ORDER-1/confirm", { received: true }));
    expect(prompt).not.toHaveBeenCalled();
  });
  it("only offers receipt confirmation for eligible bank orders", () => {
    const bank = record(1, { status: "pending", sepay: { flow: "bank", credits: 600, amount_vnd: 500000, verification_status: "pending" } });
    expect(canConfirmBank(bank)).toBe(true);
    expect(canConfirmBank({ ...bank, status: "success" })).toBe(false);
    expect(canConfirmBank({ ...bank, sepay: { ...bank.sepay!, flow: "gateway" } })).toBe(false);
    expect(canConfirmBank({ ...bank, status: "expired", sepay: { ...bank.sepay!, verification_status: "voided", verification_reason: "QR payment expired after 15 minutes" } })).toBe(true);
    expect(canConfirmBank({ ...bank, status: "expired", sepay: { ...bank.sepay!, verification_status: "voided", verification_reason: "cancelled by user" } })).toBe(false);
  });
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
    const sepayReview = record(3, {
      status: "pending",
      sepay: { credits: 100, amount_vnd: 100000, verification_status: "review" },
    });
    expect(needsReview(sepayReview)).toBe(true);
    expect(needsReview({ ...sepayReview, status: "success" })).toBe(false);
    expect(
      needsReview(record(4, { status: "pending", sepay: { credits: 100, amount_vnd: 100000, verification_status: "pending" } })),
    ).toBe(false);
    expect(formatUsdt("10.200000", "vi-VN")).toBe("10,2");
    expect(formatUsdt("10.200001", "en-US")).toBe("10.200001");
    expect(formatUsdt("100", "en-US")).toBe("100");
  });

  it("requests one server page with the query and local-day bounds", async () => {
    mocks.getAll.mockImplementation(serve([record(1)]));
    const controller = new AbortController();
    const response = await loadTopupPage(
      { page: 2, pageSize: 20, keyword: "ORDER-1", status: "review", paymentMethod: "binance", time: "all" },
      controller.signal,
    );
    expect(response.items).toHaveLength(0);
    expect(mocks.getAll).toHaveBeenCalledTimes(1);
    const [params, config] = mocks.getAll.mock.calls[0];
    expect(params).toMatchObject({ page: 2, pageSize: 20, keyword: "ORDER-1", status: "needs_review", paymentMethod: "binance" });
    expect(params.startTimestamp).toBeUndefined();
    expect(params.todayEnd).toBeGreaterThan(params.todayStart);
    expect(config).toMatchObject({ signal: controller.signal });
  });

  it("fails explicitly on an error response or an aborted request", async () => {
    mocks.getAll.mockResolvedValueOnce({ success: false });
    await expect(loadTopupPage({ page: 1, pageSize: 10, keyword: "", status: "all", paymentMethod: "all", time: "all" })).rejects.toThrow("Failed to load");
    const controller = new AbortController();
    controller.abort();
    mocks.getAll.mockClear();
    await expect(loadTopupPage({ page: 1, pageSize: 10, keyword: "", status: "all", paymentMethod: "all", time: "all" }, controller.signal)).rejects.toThrow();
    expect(mocks.getAll).not.toHaveBeenCalled();
  });

  it("computes local-day bounds from the local calendar day", () => {
    const bounds = localDayBounds(new Date(2026, 10, 18, 12, 30));
    expect(bounds.start).toBe(Math.floor(new Date(2026, 10, 18).getTime() / 1000));
    expect(bounds.end).toBe(Math.floor(new Date(2026, 10, 19).getTime() / 1000));
  });

  it("shows summary cards from the server summary fields", async () => {
    mocks.getAll.mockResolvedValue({
      success: true,
      data: {
        items: [record(1)],
        total: 1,
        summary: { ...zeroSummary, collected_vnd: 800000, today_vnd: 500000, processing: 1, needs_review: 2 },
      },
    });
    mocks.get.mockResolvedValue({ data: { success: false } });

    render(<QueryClientProvider client={new QueryClient()}><TopupManagementTable /></QueryClientProvider>);

    await waitFor(() => {
      expect(screen.getByText("Collected").parentElement?.textContent).toContain("800,000 VND");
      expect(screen.getByText("Today").parentElement?.textContent).toContain("500,000 VND");
      expect(screen.getByText("Processing").parentElement?.textContent).toContain("1");
      expect(screen.getByText("Manual review required").parentElement?.parentElement?.textContent).toContain("2");
    });
  });

  it("keeps chronological order without pinning review rows and supports review filters", async () => {
    const items = [...Array.from({ length: 11 }, (_, i) => record(i + 1)), expiredBinance];
    mocks.getAll.mockImplementation(serve(items, zeroSummary));
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
    await waitFor(() => expect(within(table).getAllByRole("row")).toHaveLength(3));
    expect(within(table).getAllByRole("row")[2].textContent).toContain("customer-99");
    fireEvent.click(screen.getByRole("button", { name: "View now →" }));
    await waitFor(() => expect(within(table).getAllByRole("row")).toHaveLength(2));
    expect(mocks.getAll).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 1, status: "needs_review" }),
      expect.anything(),
    );
    expect(within(table).getAllByRole("row")[1].textContent).toContain("customer-99");
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

  it("offers bank confirmation but not Binance actions on a SePay review row", async () => {
    const sepayReview = record(5, {
      status: "pending",
      sepay: { credits: 150, amount_vnd: 150000, verification_status: "review", flow: "bank" },
    } as Partial<TopupRecord>);
    mocks.getAll.mockImplementation(serve([sepayReview], zeroSummary));
    mocks.get.mockResolvedValue({ data: { success: false } });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={client}><TopupManagementTable /></QueryClientProvider>);
    await screen.findByText("customer-5");
    expect(screen.getByRole("button", { name: "Confirm bank receipt" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Reconcile" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Review" })).toBeNull();
    client.clear();
  });

  it("sends the order-ID keyword to the server and returns to the first page", async () => {
    const items = Array.from({ length: 25 }, (_, i) => record(i + 1));
    mocks.getAll.mockImplementation(serve(items, zeroSummary));
    mocks.get.mockResolvedValue({ data: { success: false } });
    render(<QueryClientProvider client={new QueryClient()}><TopupManagementTable /></QueryClientProvider>);
    await screen.findByText("customer-25");
    fireEvent.click(screen.getByRole("button", { name: "Next page" }));
    await screen.findByText("customer-15");
    fireEvent.change(screen.getByRole("textbox", { name: "Search order ID or TxHash" }), {
      target: { value: " ORDER-7 " },
    });
    await waitFor(() =>
      expect(mocks.getAll).toHaveBeenLastCalledWith(
        expect.objectContaining({ page: 1, pageSize: 10, keyword: "ORDER-7" }),
        expect.anything(),
      ),
    );
    const table = screen.getByRole("table");
    await waitFor(() => expect(within(table).getAllByRole("row")).toHaveLength(2));
    expect(within(table).getByText("customer-7")).toBeTruthy();
    expect(screen.getByText("Showing 1–1 / 1 transactions")).toBeTruthy();
  });

  it("requests only the visible server page instead of loading every page", async () => {
    const items = Array.from({ length: 25 }, (_, i) => record(i + 1));
    mocks.getAll.mockImplementation(serve(items, zeroSummary));
    mocks.get.mockResolvedValue({ data: { success: false } });
    render(<QueryClientProvider client={new QueryClient()}><TopupManagementTable /></QueryClientProvider>);
    await screen.findByText("customer-25");
    const requestedPages = () => mocks.getAll.mock.calls.map(([params]) => params.page);
    expect(requestedPages()).toEqual([1]);
    fireEvent.click(screen.getByRole("button", { name: "Next page" }));
    await screen.findByText("customer-15");
    expect(mocks.getAll).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 2, pageSize: 10 }),
      expect.anything(),
    );
    expect(requestedPages()).toEqual([1, 2]);
    // No request ever asks for more than one page of rows, and there is no bulk aggregate.
    expect(mocks.getAll.mock.calls.every(([params]) => params.pageSize === 10)).toBe(true);
  });

  it("shows an alert when the list request fails", async () => {
    mocks.getAll.mockResolvedValue({ success: false });
    mocks.get.mockResolvedValue({ data: { success: false } });
    render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><TopupManagementTable /></QueryClientProvider>);
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Failed to load billing history");
  });
});
