import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Copy, Loader2 } from "lucide-react";
import { QRCodeSVG } from "qrcode.react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { api } from "@/lib/api";

type Network = "BEP20" | "TRC20";
type Quote = {
  fee_percent: string;
  fee_usdt: string;
  payable_usdt: string;
  usdt_to_vnd: string;
  bonus_percent: string;
  credits: number;
};
type Order = {
  trade_no: string;
  network: Network;
  address: string;
  amount_usdt: string;
  status: string;
  expires_at: string;
  qr_payload: string;
};

const shorten = (value: string) =>
  value.length > 18 ? `${value.slice(0, 10)}…${value.slice(-8)}` : value;

// Binance matching keeps six decimal places server-side. For people reading the
// checkout, remove insignificant zeroes while preserving the exact value.
const formatUsdt = (value: string | number, locale: string) => {
  const raw = String(value).trim();
  const [integer, fraction = ""] = raw.split(".");
  const normalized = `${integer || "0"}.${fraction.replace(/0+$/, "")}`.replace(/\.$/, "");
  return locale.startsWith("vi") ? normalized.replace(".", ",") : normalized;
};

export function BinanceUsdtTopup({ onPaid }: { onPaid: () => void }) {
  const { t, i18n } = useTranslation();
  const [network, setNetwork] = useState<Network>("BEP20");
  const [amount, setAmount] = useState("10");
  const [quote, setQuote] = useState<Quote | null>(null);
  const [order, setOrder] = useState<Order | null>(null);
  const [config, setConfig] = useState<{
    min: string;
    max: string;
    networks: Record<string, boolean>;
  } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [seconds, setSeconds] = useState(0);
  const notified = useRef("");
  const locale = i18n.language.startsWith("vi") ? "vi-VN" : "en-US";
  const number = (value: string | number) => Number(value).toLocaleString(locale);
  const enabled = (item: Network) => config?.networks[item] === true;
  const statusLabel = (status: string) =>
    ({
      pending: t("Waiting for payment"),
      matched: t("Confirming network"),
      paid: t("Credit added"),
      review: t("Manual review required"),
      partial: t("Insufficient amount"),
      expired: t("Expired"),
      voided: t("Cancelled"),
    })[status] ?? status;

  useEffect(() => {
    void api
      .get("/api/user/topup/info")
      .then(({ data }) => {
        const value = data?.data;
        if (!value) return;
        const networks = value.binance_networks ?? {};
        setConfig({
          min: value.binance_min_usdt ?? "10",
          max: value.binance_max_usdt ?? "100",
          networks,
        });
        const firstEnabled = (["BEP20", "TRC20"] as Network[]).find(
          (item) => networks[item] === true,
        );
        if (firstEnabled) setNetwork(firstEnabled);
      })
      .catch(() => setError(t("Unable to load USDT configuration.")));
  }, [t]);

  useEffect(() => {
    if (!config || !amount) return;
    const timer = window.setTimeout(() => {
      void api
        .post("/api/user/binance/quote", { amount_usdt: amount })
        .then(({ data }) => setQuote(data?.data ?? null))
        .catch(() => setQuote(null));
    }, 250);
    return () => window.clearTimeout(timer);
  }, [amount, config]);

  useEffect(() => {
    if (!order || ["paid", "voided", "expired"].includes(order.status)) return;
    const tick = async () => {
      try {
        const response = await api.get(
          `/api/user/binance/orders/${encodeURIComponent(order.trade_no)}`,
        );
        if (!response.data?.success) return;
        const next = response.data.data as Order;
        setOrder((current) => ({ ...current!, ...next }));
        if (next.status === "paid" && notified.current !== next.trade_no) {
          notified.current = next.trade_no;
          onPaid();
        }
      } catch {
        setError(t("Unable to check Binance order status."));
      }
    };
    void tick();
    const timer = window.setInterval(() => void tick(), 5000);
    return () => window.clearInterval(timer);
  }, [onPaid, order?.status, order?.trade_no, t]);

  useEffect(() => {
    if (!order?.expires_at || order.status === "paid") return;
    const tick = () =>
      setSeconds(Math.max(0, Math.ceil((Date.parse(order.expires_at) - Date.now()) / 1000)));
    tick();
    const timer = window.setInterval(tick, 1000);
    return () => window.clearInterval(timer);
  }, [order?.expires_at, order?.status]);

  async function create() {
    setBusy(true);
    setError("");
    try {
      const response = await api.post("/api/user/binance/orders", {
        network,
        amount_usdt: amount,
        request_id: crypto.randomUUID(),
      });
      if (!response.data?.success) throw new Error("rejected");
      setOrder(response.data.data as Order);
    } catch {
      setError(t("Unable to create Binance order. Please try again."));
    } finally {
      setBusy(false);
    }
  }

  async function cancel() {
    if (!order || !window.confirm(t("Cancel this top-up order?"))) return;
    setBusy(true);
    try {
      const response = await api.post(
        `/api/user/binance/orders/${encodeURIComponent(order.trade_no)}/cancel`,
      );
      if (!response.data?.success) throw new Error("rejected");
      setOrder((current) => (current ? { ...current, status: "voided" } : current));
    } catch {
      setError(t("Unable to cancel Binance order."));
    } finally {
      setBusy(false);
    }
  }

  const presets = ["10", "20", "50", "100"].filter(
    (value) => !config || Number(value) <= Number(config.max),
  );

  return (
    <section className="grid gap-5 rounded-2xl border bg-card p-5 lg:grid-cols-2">
      <div className="space-y-5">
        <div>
          <h2 className="text-lg font-semibold">USDT</h2>
          <p className="text-sm text-muted-foreground">
            {t("Choose the amount and transfer network.")}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {presets.map((value) => (
            <Button
              key={value}
              type="button"
              variant={amount === value ? "default" : "outline"}
              onClick={() => setAmount(value)}
            >
              {value} USDT
            </Button>
          ))}
        </div>
        <label className="block text-sm font-medium">
          {t("USDT amount")}
          <span className="float-right text-muted-foreground">
            {t("Min")} {config?.min ?? "10"} · {t("Max")} {config?.max ?? "100"}
          </span>
          <div className="relative mt-2">
            <Input
              inputMode="decimal"
              value={amount}
              onChange={(event) => setAmount(event.target.value)}
              className="pr-16"
            />
            <span className="absolute right-3 top-2.5 text-sm text-muted-foreground">USDT</span>
          </div>
        </label>
        <div>
          <div className="mb-2 text-sm font-medium">{t("Transfer network")}</div>
          <div className="grid gap-3 sm:grid-cols-2">
            {(["BEP20", "TRC20"] as Network[]).map((item) => (
              <button
                key={item}
                type="button"
                disabled={!enabled(item)}
                onClick={() => setNetwork(item)}
                className={`rounded-xl border p-4 text-left disabled:cursor-not-allowed disabled:opacity-50 ${network === item ? "border-primary ring-1 ring-primary" : "border-border"}`}
              >
                <div className="font-semibold">{item}</div>
                <div className="text-sm text-muted-foreground">
                  {item === "BEP20" ? "BNB Smart Chain" : "TRON"}
                </div>
              </button>
            ))}
          </div>
          <p className="mt-2 text-sm text-amber-500">
            {t("Sending on the wrong network may result in permanent loss.")}
          </p>
        </div>
        <div className="divide-y rounded-xl border bg-background/40 px-4">
          {[
            [t("Exchange rate"), quote ? `1 USDT = ${number(quote.usdt_to_vnd)} VND` : "—"],
            [
              t("Payment fee"),
              quote
                ? `${formatUsdt(quote.fee_percent, locale)}% (${formatUsdt(quote.fee_usdt, locale)} USDT)`
                : "—",
            ],
            [
              t("Total to send"),
              quote ? `${formatUsdt(quote.payable_usdt, locale)} USDT` : "—",
            ],
            [t("Bonus"), quote ? `${quote.bonus_percent}%` : "—"],
            [t("You receive"), quote ? `${number(quote.credits)} credit` : "—"],
          ].map(([label, value]) => (
            <div key={label} className="flex justify-between py-3 text-sm">
              <span className="text-muted-foreground">{label}</span>
              <strong>{value}</strong>
            </div>
          ))}
        </div>
        <Button
          className="w-full"
          onClick={() => void create()}
          disabled={busy || !quote || !enabled(network)}
        >
          {busy && <Loader2 className="mr-2 size-4 animate-spin" />}
          {t("Create USDT top-up order")}
        </Button>
        {error && <p className="text-sm text-destructive">{error}</p>}
      </div>
      <div className="min-h-[360px] rounded-2xl border bg-background/40 p-5">
        {order ? (
          <div className="space-y-4">
            <div className="flex items-start justify-between">
              <div>
                <h3 className="font-semibold">
                  {t("Order")} {order.trade_no.slice(0, 8)}…{order.trade_no.slice(-4)}
                </h3>
                <p className="text-sm text-muted-foreground">
                  {t("Time remaining")} {Math.floor(seconds / 60)}:
                  {String(seconds % 60).padStart(2, "0")}
                </p>
              </div>
              <span className="rounded-full bg-primary/20 px-3 py-1 text-xs font-semibold text-primary">
                {statusLabel(order.status)}
              </span>
            </div>
            {!["paid", "voided", "expired"].includes(order.status) && (
              <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => void cancel()}>
                {t("Cancel order")}
              </Button>
            )}
            <div className="flex flex-col gap-4 sm:flex-row">
              <div className="rounded-xl bg-white p-3">
                <QRCodeSVG value={order.qr_payload} size={150} />
              </div>
              <div className="space-y-3 text-sm">
                <div>
                  <span className="text-muted-foreground">{t("Send exactly")}</span>
                  <div className="text-xl font-bold">
                    {formatUsdt(order.amount_usdt, locale)} USDT
                  </div>
                </div>
                <div>
                  <span className="text-muted-foreground">{t("Network")}</span>
                  <div className="font-semibold">{order.network}</div>
                </div>
                <div className="flex items-center gap-1 font-mono">
                  {shorten(order.address)}
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={t("Copy address")}
                    onClick={() => void navigator.clipboard?.writeText(order.address)}
                  >
                    <Copy className="size-4" />
                  </Button>
                </div>
              </div>
            </div>
            <div className="grid grid-cols-3 gap-2 border-t pt-4 text-center text-xs">
              <div className="border-t-2 border-primary pt-2">{t("Waiting for transaction")}</div>
              <div className="border-t-2 border-muted pt-2">{t("Network confirmation")}</div>
              <div className="border-t-2 border-muted pt-2">{t("Credit settlement")}</div>
            </div>
          </div>
        ) : (
          <div className="flex min-h-[320px] items-center justify-center text-center text-muted-foreground">
            {t("Your top-up order details will appear here.")}
          </div>
        )}
      </div>
    </section>
  );
}
