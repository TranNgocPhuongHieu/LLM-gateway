import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { BinanceUsdtTopup, usdtBonusPercent } from "./binance-usdt-topup";

const mocks = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }));
// react-i18next returns a stable t; an unstable mock would re-run effects forever.
const i18n = vi.hoisted(() => ({ t: (key: string) => key, i18n: { language: "vi" } }));
vi.mock("@/lib/api", () => ({ api: mocks }));
vi.mock("react-i18next", () => ({ useTranslation: () => i18n }));
vi.mock("qrcode.react", () => ({
  QRCodeSVG: ({ value }: { value: string }) => <div data-testid="qr">{value}</div>,
}));

const address = "0x00000000000000000000000000000000000000b1";
const topupInfo = {
  data: {
    success: true,
    data: { binance_min_usdt: "10", binance_max_usdt: "1000", binance_networks: { BEP20: true, TRC20: true } },
  },
};

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("USDT checkout", () => {
  it("shows the quote failure instead of leaving empty placeholders", async () => {
    mocks.get.mockResolvedValue(topupInfo);
    mocks.post.mockResolvedValue({ data: { success: false, message: "Binance top-up is unavailable" } });
    render(<BinanceUsdtTopup onPaid={() => {}} />);
    expect(await screen.findByText("Binance top-up is unavailable")).toBeTruthy();
    expect((screen.getByRole("button", { name: "Create USDT top-up order" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("encodes only the address in the QR and copies the exact amount", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
    mocks.get.mockImplementation((path: string) =>
      Promise.resolve(path === "/api/user/topup/info" ? topupInfo : { data: { success: true, data: { status: "pending" } } }),
    );
    mocks.post.mockImplementation((path: string) =>
      Promise.resolve(
        path === "/api/user/binance/quote"
          ? { data: { success: true, data: { fee_percent: "2", fee_usdt: "0.2", payable_usdt: "10.200000", usdt_to_vnd: "26000", bonus_percent: "0", credits: 260 } } }
          : { data: { success: true, data: { trade_no: "BNB-ORDER-0001", network: "BEP20", address, amount_usdt: "10.200000", status: "pending", expires_at: new Date(Date.now() + 900000).toISOString(), qr_payload: address } } },
      ),
    );
    render(<BinanceUsdtTopup onPaid={() => {}} />);
    const create = await screen.findByRole("button", { name: "Create USDT top-up order" });
    await waitFor(() => expect((create as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(create);

    expect((await screen.findByTestId("qr")).textContent).toBe(address);
    fireEvent.click(screen.getByRole("button", { name: "Copy amount" }));
    expect(writeText).toHaveBeenCalledWith("10.2");
  });

  it("shows the bonus tiers and the bonus credits on each preset", async () => {
    const tiers = [
      { min_usdt: "10", max_usdt: "20", bonus_percent: "0" },
      { min_usdt: "20", max_usdt: "50", bonus_percent: "10" },
      { min_usdt: "50", max_usdt: "100", bonus_percent: "15" },
      { min_usdt: "100", bonus_percent: "20" },
    ];
    mocks.get.mockResolvedValue({
      data: { success: true, data: { ...topupInfo.data.data, binance_usdt_to_vnd: "26000", binance_bonus_tiers: tiers } },
    });
    mocks.post.mockResolvedValue({ data: { success: false, message: "x" } });
    render(<BinanceUsdtTopup onPaid={() => {}} />);
    expect((await screen.findByTestId("usdt-bonus-tiers")).textContent).toBe(
      "Bonus tiers: from 20 USDT +10% · from 50 USDT +15% · from 100 USDT +20%",
    );
    const label = (name: string) => screen.getByRole("button", { name: new RegExp(`^${name} USDT`) }).textContent;
    expect(label("10")).toBe("10 USDT");
    expect(label("20")).toBe("20 USDT+52 credit bonus");
    expect(label("50")).toBe("50 USDT+195 credit bonus");
    expect(label("100")).toBe("100 USDT+520 credit bonus");
  });

  it("picks the bonus tier like the server", () => {
    const tiers = [
      { min_usdt: "10", max_usdt: "20", bonus_percent: "0" },
      { min_usdt: "20", max_usdt: "50", bonus_percent: "10" },
      { min_usdt: "100", bonus_percent: "20" },
    ];
    expect(usdtBonusPercent(9, tiers)).toBe(0);
    expect(usdtBonusPercent(19.99, tiers)).toBe(0);
    expect(usdtBonusPercent(20, tiers)).toBe(10);
    expect(usdtBonusPercent(60, tiers)).toBe(0);
    expect(usdtBonusPercent(1000, tiers)).toBe(20);
    expect(usdtBonusPercent(30, [])).toBe(0);
  });
});
