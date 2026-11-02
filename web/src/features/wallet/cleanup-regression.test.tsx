// @vitest-environment jsdom
import { render, screen, waitFor, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { getSelf } from "@/lib/api";
import { Wallet } from "./index";
import { requestPaymentAmount } from "./hooks/use-payment";
import { getDefaultPaymentType, getMinTopupAmount } from "./lib/payment";
const mocks = vi.hoisted(() => ({
  process: vi.fn(async () => true),
  calculate: vi.fn(async () => 10),
  history: vi.fn(),
}));
vi.mock("@/lib/api", () => ({
  getSelf: vi.fn(async () => ({ success: true, data: { quota: 456 } })),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'vi' } }),
}));
vi.mock("@/hooks/use-status", () => ({
  useStatus: () => ({ status: { price: 1 } }),
}));
vi.mock("@/hooks/use-system-config", () => ({
  useSystemConfig: () => ({ currency: { quotaDisplayType: "USD" } }),
}));
vi.mock("./hooks", () => ({
  useTopupInfo: () => ({
    topupInfo: {
      enable_stripe_topup: true,
      stripe_min_topup: 10,
      pay_methods: [],
    },
    presetAmounts: [],
    loading: false,
  }),
  usePayment: () => ({
    amount: 10,
    calculating: false,
    processing: false,
    calculatePaymentAmount: mocks.calculate,
    processPayment: mocks.process,
  }),
}));
vi.mock("@/components/layout", () => ({
  SectionPageLayout: Object.assign(
    ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
    {
      Title: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
      Content: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
    },
  ),
}));
vi.mock("./components/wallet-stats-card", () => ({
  WalletStatsCard: ({ user }: { user: { quota: number } | null }) => (
    <div>Balance: {user?.quota ?? "unknown"}</div>
  ),
}));
vi.mock("./components/recharge-form-card", () => ({
  RechargeFormCard: ({
    onPaymentMethodSelect,
  }: {
    onPaymentMethodSelect: (value: { type: string; name: string }) => void;
  }) => (
    <button onClick={() => onPaymentMethodSelect({ type: "stripe", name: "Stripe" })}>
      Select Stripe
    </button>
  ),
}));
vi.mock("./components/dialogs/payment-confirm-dialog", () => ({
  PaymentConfirmDialog: ({ open, onConfirm }: { open: boolean; onConfirm: () => void }) =>
    open ? <button onClick={onConfirm}>Confirm</button> : null,
}));
vi.mock("./components/dialogs/billing-history-dialog", () => ({
  BillingHistory: ({ refreshKey }: { refreshKey: number }) => <section id='wallet-history'>Inline top-up history: {refreshKey}</section>,
}));
vi.mock('./hooks/use-billing-history', () => ({ useBillingHistory: mocks.history }));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
Object.defineProperty(window, 'matchMedia', { writable: true, value: () => ({ matches: false, addListener: vi.fn(), removeListener: vi.fn(), addEventListener: vi.fn(), removeEventListener: vi.fn(), dispatchEvent: vi.fn() }) });

describe("wallet cleanup preserves supported payment paths", () => {
  it("loads balance and confirms Stripe with amount first, then payment type", async () => {
    render(<Wallet />);
    await screen.findByText("Balance: 456");
    expect(getSelf).toHaveBeenCalledTimes(1);
    await userEvent.click(screen.getByRole("button", { name: "Select Stripe" }));
    await userEvent.click(await screen.findByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(mocks.process).toHaveBeenCalledWith(10, "stripe"));
    await waitFor(() => expect(getSelf).toHaveBeenCalledTimes(2));
    expect(screen.getByText('Inline top-up history: 1')).toBeTruthy();
  });
  it("keeps Epay and Stripe amount calculators separate", async () => {
    const calculators = {
      regular: vi.fn(async () => ({ success: true, data: "7.3" })),
      stripe: vi.fn(async () => ({ success: true, data: "8" })),
    };
    expect(await requestPaymentAmount(100, "alipay", calculators)).toBe(7.3);
    expect(await requestPaymentAmount(100, "stripe", calculators)).toBe(8);
    expect(calculators.regular).toHaveBeenCalledWith({ amount: 100 });
    expect(calculators.stripe).toHaveBeenCalledWith({ amount: 100 });
  });
  it("keeps supported defaults and does not revive removed gateway calculators", () => {
    expect(getDefaultPaymentType(null)).toBe("alipay");
    expect(getMinTopupAmount(null)).toBe(1);
  });
  it("does not render subscription purchasing", async () => {
    render(<Wallet />);
    await screen.findByText("Balance: 456");
    expect(screen.queryByText("Subscription Plans")).toBeNull();
  });
  it('renders SePay history inline with VND and credits, without admin controls for users', async () => {
    const refresh = vi.fn();
    mocks.history.mockReturnValue({
      records: [{ id: 1, user_id: 7, trade_no: 'SPfixture', payment_provider: 'sepay', payment_method: 'sepay_bank_transfer', amount: 100, money: 100000, status: 'pending', create_time: 1792500000, sepay: { credits: 100, amount_vnd: 100000 } }],
      total: 1, page: 1, pageSize: 10, keyword: '', loading: false, completing: false, isAdmin: false,
      handlePageChange: vi.fn(), handlePageSizeChange: vi.fn(), handleSearch: vi.fn(), handleCompleteOrder: vi.fn(), refresh,
    });
    const { BillingHistory } = await vi.importActual<typeof import('./components/dialogs/billing-history-dialog')>('./components/dialogs/billing-history-dialog');
    render(<BillingHistory refreshKey={3} />);
    expect(screen.getByRole('heading', { name: 'Top-up History' })).toBeTruthy();
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(screen.getByText('100.000 VND')).toBeTruthy();
    expect(screen.getByText('100 credit')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Reconcile Payment' })).toBeNull();
    expect(mocks.history).toHaveBeenCalledWith({ refreshKey: 3 });
    await userEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    expect(refresh).toHaveBeenCalledTimes(1);
    cleanup();
    mocks.history.mockReturnValue({ ...mocks.history.mock.results[0].value, isAdmin: true });
    render(<BillingHistory />);
    expect(screen.queryByRole('button', { name: 'Complete Order' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Reconcile Payment' })).toBeNull();
    expect(screen.getByText('SePay will verify this payment automatically after IPN confirmation.')).toBeTruthy();
  });
});
