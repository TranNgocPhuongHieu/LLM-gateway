import { useCallback, useEffect, useRef, useState } from "react";
import { Check, Copy, QrCode, Timer, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { api } from "@/lib/api";
import { normalizeInterfaceLanguage } from "@/i18n/languages";
import { useIsAdmin } from "@/hooks/use-admin";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { cancelSepayOrder } from "../api";

export const SEPAY_PRESETS = [10_000, 100_000, 200_000, 500_000, 1_000_000];
export const validSepayAmount = (amount: number) =>
  Number.isInteger(amount) &&
  amount >= 10_000 &&
  amount <= 1_000_000 &&
  amount % 1000 === 0;
export const sepayBonusPercent = (amount: number) =>
  amount >= 1_000_000
    ? 30
    : amount >= 500_000
      ? 20
      : amount >= 200_000
        ? 15
        : amount >= 100_000
          ? 10
          : 0;
const bonusPercentFor = sepayBonusPercent;
type Order = {
  trade_no: string;
  amount_vnd: number;
  credits: number;
  status: string;
  flow?: string;
  qr_url?: string;
  bank_code?: string;
  bank_account?: string;
  bank_holder?: string;
  transfer_content?: string;
  expires_at?: string;
};

export function SepayTopup({
  userId,
  onPaid,
  onCreated,
  selectedInvoice,
}: {
  userId: number;
  onPaid: () => void;
  onCreated?: () => void;
  selectedInvoice?: string;
}) {
  const { i18n } = useTranslation();
  const vi =
    normalizeInterfaceLanguage(i18n.resolvedLanguage || i18n.language) === "vi";
  const s = (vn: string, en: string) => (vi ? vn : en);
  const format = (n: number) =>
    new Intl.NumberFormat(vi ? "vi-VN" : "en-US").format(n);
  const [enabled, setEnabled] = useState(false);
  const [configLoading, setConfigLoading] = useState(true);
  const isAdmin = useIsAdmin();
  const [rawAmount, setRawAmount] = useState("10000");
  const amount = Number(rawAmount);
  const bonusPercent = sepayBonusPercent(amount);
  const creditedAmount = amount + (amount * bonusPercent) / 100;
  const [processing, setProcessing] = useState(false);
  const [error, setError] = useState("");
  const [order, setOrder] = useState<Order | null>(null);
  const [invoice, setInvoice] = useState("");
  const [polling, setPolling] = useState(false);
  const [secondsLeft, setSecondsLeft] = useState(0);
  const [cancelling, setCancelling] = useState(false);
  const request = useRef<{ amount: number; id: string } | null>(null);
  const notified = useRef("");
  const paidCallback = useRef(onPaid);
  paidCallback.current = onPaid;
  const { copyToClipboard, copiedText } = useCopyToClipboard({ notify: false });

  const paid = order?.status === "paid";
  const pending =
    !!invoice &&
    !paid &&
    order?.status !== "review" &&
    order?.status !== "voided" &&
    order?.status !== "expired";

  useEffect(() => {
    let active = true;
    setConfigLoading(true);
    void api
      .get("/api/user/sepay/config")
      .then(({ data }) => {
        if (!data.success) throw new Error("configuration unavailable");
        if (active)
          setEnabled(data.data.enabled && data.data.bank_enabled !== false);
      })
      .catch(() => {
        if (active)
          setError(
            s(
              "Không tải được cấu hình SePay.",
              "Could not load SePay configuration.",
            ),
          );
      })
      .finally(() => {
        if (active) setConfigLoading(false);
      });
    const candidate =
      selectedInvoice ||
      new URLSearchParams(window.location.search).get("trade_no") ||
      sessionStorage.getItem(`sepay-active:${userId}`);
    if (candidate && /^(SP[a-zA-Z0-9]{28}|NAP[A-Z0-9]{28})$/.test(candidate)) {
      setOrder(null);
      setInvoice(candidate);
      setPolling(true);
    }
    return () => {
      active = false;
    };
  }, [userId, selectedInvoice]);

  const check = useCallback(
    async (trade: string, active: () => boolean) => {
      try {
        const { data } = await api.get(
          `/api/user/sepay/orders/${encodeURIComponent(trade)}`,
        );
        if (!data.success) throw new Error("status unavailable");
        if (!active()) return;
        const next = data.data as Order;
        setOrder((previous) => ({ ...previous, ...next }));
        if (next.status === "paid") {
          setPolling(false);
          sessionStorage.removeItem(
            `sepay-bank-request:${userId}:${next.amount_vnd}`,
          );
          sessionStorage.removeItem(`sepay-active:${userId}`);
          request.current = null;
          if (notified.current !== trade) {
            notified.current = trade;
            paidCallback.current();
          }
        } else if (
          next.status === "voided" ||
          next.status === "expired" ||
          next.status === "review"
        )
          setPolling(false);
        setError("");
      } catch {
        if (active())
          setError(
            s(
              "Chưa kiểm tra được trạng thái. Không thanh toán thêm nếu đã chuyển tiền.",
              "Could not check payment status. Do not pay again if already paid.",
            ),
          );
      }
    },
    [vi, userId],
  );

  useEffect(() => {
    if (!invoice || !polling) return;
    let active = true;
    let busy = false;
    const tick = async () => {
      if (!active || busy || document.hidden) return;
      busy = true;
      try {
        await check(invoice, () => active);
      } finally {
        busy = false;
      }
    };
    void tick();
    const interval = window.setInterval(() => void tick(), 4000);
    const timeout = window.setTimeout(() => {
      setPolling(false);
      setOrder((current) =>
        current ? { ...current, status: "voided" } : current,
      );
      setError(
        s(
          "Mã QR đã hết hạn sau 15 phút. Hãy tạo mã mới nếu chưa chuyển tiền.",
          "This QR expired after 15 minutes. Create a new one if you have not paid.",
        ),
      );
    }, 900000);
    return () => {
      active = false;
      window.clearInterval(interval);
      window.clearTimeout(timeout);
    };
  }, [invoice, polling, check]);

  useEffect(() => {
    if (!order?.expires_at || !pending) {
      setSecondsLeft(0);
      return;
    }
    const update = () => {
      const remaining = Math.max(0, Math.ceil((Date.parse(order.expires_at!) - Date.now()) / 1000));
      setSecondsLeft(remaining);
      if (remaining === 0) setPolling(false);
    };
    update();
    const interval = window.setInterval(update, 1000);
    return () => window.clearInterval(interval);
  }, [order?.expires_at, pending]);

  async function cancelOrder() {
    if (!invoice || cancelling || !pending) return;
    setCancelling(true);
    try {
      const response = await cancelSepayOrder(invoice);
      if (!response.success) throw new Error("cancel failed");
      setPolling(false);
      setOrder((current) => current ? { ...current, status: "voided" } : current);
      sessionStorage.removeItem(`sepay-active:${userId}`);
      sessionStorage.removeItem(`sepay-bank-request:${userId}:${order?.amount_vnd ?? amount}`);
      request.current = null;
    } catch {
      setError(s("Không thể huỷ mã QR lúc này. Vui lòng thử lại.", "Could not cancel this QR yet. Please try again."));
    } finally {
      setCancelling(false);
    }
  }

  async function checkout() {
    if (!enabled || !validSepayAmount(amount) || processing) return;
    setProcessing(true);
    setError("");
    try {
      const storageKey = `sepay-bank-request:${userId}:${amount}`;
      if (request.current?.amount !== amount)
        request.current = {
          amount,
          id: sessionStorage.getItem(storageKey) || crypto.randomUUID(),
        };
      sessionStorage.setItem(storageKey, request.current.id);
      const { data } = await api.post("/api/user/sepay/pay", {
        amount_vnd: amount,
        request_id: request.current.id,
        flow: "bank",
      });
      if (!data.success) {
        setError(
          data.message || s("Không tạo được mã QR.", "Could not create QR."),
        );
        return;
      }
      const next = data.data as Order;
      if (next.flow !== "bank") throw new Error("direct QR unavailable");
      if (next.status !== "paid") {
        const url = new URL(next.qr_url || "");
        if (url.origin !== "https://vietqr.app" || url.pathname !== "/img")
          throw new Error("invalid QR source");
      }
      setOrder(next);
      setInvoice(next.trade_no);
      sessionStorage.setItem(`sepay-active:${userId}`, next.trade_no);
      setPolling(true);
      onCreated?.();
    } catch {
      setError(
        s(
          "Không tạo được mã QR. Thử lại để dùng cùng mã yêu cầu, tránh tạo đơn trùng.",
          "Could not create QR. Retry to reuse the request ID and avoid duplicates.",
        ),
      );
    } finally {
      setProcessing(false);
    }
  }

  const details = order
    ? [
        [s("Ngân hàng", "Bank"), order.bank_code],
        [s("Số tài khoản", "Account number"), order.bank_account],
        [s("Chủ tài khoản", "Account holder"), order.bank_holder],
        [s("Số tiền", "Amount"), `${format(order.amount_vnd)} VND`],
        [
          s("Nội dung CK", "Transfer content"),
          order.transfer_content || invoice,
        ],
      ]
    : [];

  return (
    <div
      className="grid items-start gap-5 lg:grid-cols-[minmax(0,1.3fr)_minmax(0,.85fr)]"
      data-wallet-layout="inline-qr"
    >
      <section className="rounded-2xl border bg-card p-5 sm:p-7">
        <h2 className="text-xl font-bold">
          {s("Nạp tiền bằng QR ngân hàng", "Top up with bank QR")}
        </h2>
        <p className="mt-2 text-sm text-muted-foreground">
          {s(
            "Nạp tối thiểu 10.000 VND. Mốc 100k/200k/500k/1 triệu được cộng lần lượt 10%/15%/20%/30%.",
            "Minimum top-up is VND 10,000. The 100k/200k/500k/1m tiers receive 10%/15%/20%/30% bonus.",
          )}
        </p>
        <div className="my-6 grid grid-cols-2 gap-3 sm:grid-cols-5">
          {SEPAY_PRESETS.map((preset) => (
            <button
              type="button"
              key={preset}
              aria-pressed={amount === preset}
              disabled={pending || processing}
              onClick={() => setRawAmount(String(preset))}
              className={`relative rounded-xl border px-2 py-4 text-center transition-colors disabled:opacity-60 ${amount === preset ? "border-primary bg-primary/10 ring-1 ring-primary" : "bg-muted/30 hover:border-primary/50"}`}
            >
              {preset === 100_000 && (
                <span className="absolute -top-2 right-2 rounded-full bg-primary px-2 text-[10px] text-primary-foreground">
                  {s("Phổ biến", "Popular")}
                </span>
              )}
              <span className="block font-bold">{format(preset)} VND</span>
              <span
                className={
                  bonusPercentFor(preset) > 0
                    ? "mt-1 block text-xs font-medium text-emerald-500"
                    : "text-muted-foreground mt-1 block text-xs"
                }
              >
                {bonusPercentFor(preset) > 0
                  ? `+${format((preset * bonusPercentFor(preset)) / 100)} ${s("thưởng", "bonus")}`
                  : s("Không có thưởng", "No bonus")}
              </span>
            </button>
          ))}
        </div>
        <label htmlFor="sepay-amount" className="text-sm font-semibold">
          {s("Hoặc nhập số tiền khác (VND)", "Or enter another amount (VND)")}
        </label>
        <div className="relative mt-2">
          <Input
            id="sepay-amount"
            type="number"
            min={10_000}
            max={1_000_000}
            step={1000}
            disabled={pending || processing}
            value={rawAmount}
            onChange={(event) => setRawAmount(event.target.value)}
            className="h-14 rounded-xl pr-20 text-lg font-semibold"
          />
          <span className="absolute top-1/2 right-4 -translate-y-1/2 text-sm text-muted-foreground">
            VND
          </span>
        </div>
        <div className="my-6 flex justify-between rounded-xl bg-muted/40 p-4">
          <div>
            <p className="text-xs text-muted-foreground">
              {s("Bạn nhận", "You receive")}
            </p>
            <p className="mt-1 font-bold">
              {validSepayAmount(amount) ? format(creditedAmount / 1000) : "—"}{" "}
              credit
            </p>
            {bonusPercent > 0 && (
              <p className="text-xs text-emerald-600">
                +{bonusPercent}% {s("khuyến mãi", "promotion")}
              </p>
            )}
          </div>
          <div className="text-right">
            <p className="text-xs text-muted-foreground">
              {s("Bạn thanh toán", "You pay")}
            </p>
            <p className="mt-1 font-bold">
              {validSepayAmount(amount) ? format(amount) : "—"} VND
            </p>
          </div>
        </div>
        {!validSepayAmount(amount) && (
          <p role="alert" className="mb-3 text-sm text-destructive">
            {s(
              "Nhập số tiền từ 10.000 đến 1.000.000 VND, bội số 1.000.",
              "Enter an amount from 10,000 to 1,000,000 VND in 1,000-VND steps.",
            )}
          </p>
        )}
        {!enabled && (
          <div role="status" className="mb-4 rounded-lg bg-muted p-3 text-sm">
            {configLoading
              ? s("Đang kiểm tra thanh toán…", "Checking payment availability…")
              : s(
                  "Nạp QR ngân hàng chưa được bật hoặc chưa đủ cấu hình.",
                  "Bank QR top-up is disabled or not fully configured.",
                )}
            {isAdmin && !configLoading && (
              <a
                href="/system-settings/billing/payment"
                className="mt-2 block text-primary underline"
              >
                {s("Mở cài đặt SePay", "Open SePay settings")}
              </a>
            )}
          </div>
        )}
        <Button
          type="button"
          className="h-12 w-full rounded-xl font-semibold"
          disabled={
            !enabled || !validSepayAmount(amount) || processing || pending
          }
          onClick={() => void checkout()}
        >
          {processing
            ? s("Đang tạo mã QR…", "Creating QR…")
            : s("Tạo mã QR", "Create QR")}
        </Button>
        {error && (
          <p role="alert" className="mt-3 text-sm text-destructive">
            {error}
          </p>
        )}
      </section>
      <section
        className="rounded-2xl border bg-card p-5 sm:p-7"
        aria-live="polite"
      >
        {paid ? (
          <div className="flex min-h-72 flex-col items-center justify-center text-center">
            <div className="mb-5 rounded-full bg-emerald-500/15 p-5 text-emerald-500">
              <Check className="size-8" />
            </div>
            <h3 className="text-xl font-bold">
              {s("Nạp thành công", "Top-up successful")}
            </h3>
            <p className="mt-3 text-3xl font-bold text-emerald-500">
              +{format(order.credits ?? order.amount_vnd / 1000)} credit
            </p>
            <p className="mt-2 text-sm text-muted-foreground">
              {s(
                "Credit đã được cộng vào số dư của bạn.",
                "Credits have been added to your balance.",
              )}
            </p>
            <Button
              type="button"
              className="mt-6 h-12 w-full"
              onClick={() => {
                setInvoice("");
                setOrder(null);
              }}
            >
              {s("Nạp thêm", "Top up again")}
            </Button>
          </div>
        ) : order?.qr_url ? (
          <div className="space-y-4">
            <div className="text-center">
              <span className="rounded-full bg-amber-500/15 px-3 py-1 text-xs font-semibold text-amber-600">
                {order.status === "voided"
                  ? s("Đã huỷ", "Cancelled")
                  : order.status === "expired"
                    ? s("Đã hết hạn", "Expired")
                    : order.status === "review"
                      ? s(
                          "Cần kiểm tra — không chuyển thêm",
                          "Review required — do not pay again",
                        )
                      : s("Đang chờ thanh toán", "Payment pending")}
              </span>
              <p className="mt-4 text-2xl font-bold">
                {format(order.amount_vnd)} VND
              </p>
            </div>
            {pending && (
              <img
                src={order.qr_url}
                alt={s("Mã QR chuyển khoản", "Bank transfer QR code")}
                className="mx-auto aspect-square w-full max-w-64 rounded-xl bg-white p-3"
              />
            )}
            {pending && (
              <div className="flex items-center justify-center gap-2 rounded-lg bg-primary/10 px-3 py-2 text-sm font-semibold text-primary">
                <Timer className="size-4" />
                {s("Còn lại", "Expires in")} {String(Math.floor(secondsLeft / 60)).padStart(2, "0")}:{String(secondsLeft % 60).padStart(2, "0")}
              </div>
            )}
            <p className="text-center text-sm text-muted-foreground">
              {s(
                "Mở app ngân hàng, chọn quét QR. Số tiền và nội dung đã được điền sẵn.",
                "Scan with your banking app. Amount and content are prefilled.",
              )}
            </p>
            <dl className="divide-y">
              {details.map(([label, value]) => (
                <div
                  key={label}
                  className="flex items-center justify-between gap-2 py-3 text-sm"
                >
                  <dt className="shrink-0 text-muted-foreground">{label}</dt>
                  <dd className="flex min-w-0 items-center gap-2 text-right">
                    <span className="break-all font-semibold">{value}</span>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="size-6 shrink-0"
                      aria-label={`${s("Sao chép", "Copy")} ${label}`}
                      onClick={() => void copyToClipboard(value || "")}
                    >
                      {copiedText === value ? (
                        <Check className="size-3" />
                      ) : (
                        <Copy className="size-3" />
                      )}
                    </Button>
                  </dd>
                </div>
              ))}
            </dl>
            <p className="rounded-xl bg-amber-500/10 p-3 text-xs leading-5 text-amber-600">
              {s(
                "Giữ nguyên nội dung chuyển khoản để credit được cộng tự động. Không chuyển lại nếu trạng thái chưa cập nhật.",
                "Keep the transfer content unchanged for automatic crediting. Do not pay again if status is still pending.",
              )}
            </p>
            {!polling && pending && (
              <Button
                type="button"
                variant="outline"
                className="w-full"
                onClick={() => setPolling(true)}
              >
                {s("Tiếp tục theo dõi thanh toán", "Resume payment monitoring")}
              </Button>
            )}
            {pending && (
              <Button type="button" variant="destructive" className="w-full" disabled={cancelling} onClick={() => void cancelOrder()}>
                <X className="mr-2 size-4" />
                {cancelling ? s("Đang huỷ…", "Cancelling…") : s("Huỷ mã QR", "Cancel QR")}
              </Button>
            )}
          </div>
        ) : invoice ? (
          <div className="flex min-h-72 flex-col items-center justify-center gap-3 text-center">
            <QrCode className="size-12 text-muted-foreground" />
            <p>{s("Đang kiểm tra đơn nạp tiền…", "Checking your top-up…")}</p>
            <code className="break-all text-xs">{invoice}</code>
          </div>
        ) : (
          <div className="flex min-h-72 flex-col items-center justify-center rounded-xl border border-dashed p-5 text-center">
            <QrCode className="mb-5 size-16 text-muted-foreground" />
            <h3 className="font-bold">
              {s("Mã QR sẽ hiện ngay tại đây", "Your QR code will appear here")}
            </h3>
            <p className="mt-2 max-w-xs text-sm text-muted-foreground">
              {s(
                "Chọn số credit rồi bấm “Tạo mã QR”, mở app ngân hàng và quét.",
                "Choose credits, click “Create QR”, then scan with your banking app.",
              )}
            </p>
          </div>
        )}
      </section>
    </div>
  );
}
