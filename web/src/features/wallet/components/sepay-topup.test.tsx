import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SepayTopup, sepayBonusPercent, validSepayAmount } from "./sepay-topup";

const mocks = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), admin: false }));
vi.mock("@/lib/api", () => ({ api: mocks }));
vi.mock("@/hooks/use-admin", () => ({ useIsAdmin: () => mocks.admin }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ i18n: { language: "vi" } }),
}));
vi.mock("@/hooks/use-copy-to-clipboard", () => ({
  useCopyToClipboard: () => ({ copyToClipboard: vi.fn(), copiedText: null }),
}));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  sessionStorage.clear();
  mocks.admin = false;
});

describe("inline SePay bank QR top-up", () => {
  it("validates the agreed credit range", () => {
    expect(validSepayAmount(10000)).toBe(true);
    expect(validSepayAmount(1000000)).toBe(true);
    expect(validSepayAmount(999)).toBe(false);
    expect(validSepayAmount(1500)).toBe(false);
    expect(sepayBonusPercent(100000)).toBe(10);
    expect(sepayBonusPercent(150000)).toBe(10);
    expect(sepayBonusPercent(1000000)).toBe(30);
  });
  it("renders the new two-panel wallet layout", async () => {
    mocks.get.mockResolvedValue({
      data: { success: true, data: { enabled: true, bank_enabled: true } },
    });
    render(<SepayTopup userId={1} onPaid={() => {}} />);
    await screen.findByRole("button", { name: "Tạo mã QR" });
    expect(screen.getByText("Mã QR sẽ hiện ngay tại đây")).toBeTruthy();
    expect(screen.getByLabelText("Hoặc nhập số tiền khác (VND)")).toBeTruthy();
    expect(screen.getAllByText("100.000 VND").length).toBeGreaterThan(0);
  });
  it("shows QR in the same page and polls for automatic credit", async () => {
    const onPaid = vi.fn();
    mocks.get.mockImplementation((path: string) =>
      Promise.resolve({
        data: {
          success: true,
          data: path.endsWith("/config")
            ? { enabled: true, bank_enabled: true }
            : { status: "paid", amount_vnd: 50000, credits: 50 },
        },
      }),
    );
    mocks.post.mockResolvedValue({
      data: {
        success: true,
        data: {
          flow: "bank",
          status: "pending",
          trade_no: "NAP" + "A".repeat(28),
          qr_url: "https://vietqr.app/img?bank=TPBANK",
          bank_code: "TPBANK",
          bank_account: "123456789",
          bank_holder: "FIXTURE USER",
          transfer_content: "NAP" + "A".repeat(28),
          amount_vnd: 50000,
          credits: 50,
        },
      },
    });
    render(<SepayTopup userId={1} onPaid={onPaid} />);
    const button = await screen.findByRole("button", { name: "Tạo mã QR" });
    fireEvent.click(button);
    expect(await screen.findByAltText("Mã QR chuyển khoản")).toBeTruthy();
    await waitFor(() =>
      expect(screen.getByText("Nạp thành công")).toBeTruthy(),
    );
    expect(onPaid).toHaveBeenCalledTimes(1);
  });
  it("does not enable checkout while SePay is unavailable", async () => {
    mocks.get.mockResolvedValue({
      data: { success: true, data: { enabled: false, bank_enabled: false } },
    });
    render(<SepayTopup userId={1} onPaid={() => {}} />);
    expect(
      (await screen.findByRole("button", { name: "Tạo mã QR" })).hasAttribute(
        "disabled",
      ),
    ).toBe(true);
    expect(
      screen.getByText(/chưa được bật hoặc chưa đủ cấu hình/),
    ).toBeTruthy();
    expect(mocks.post).not.toHaveBeenCalled();
  });
});
