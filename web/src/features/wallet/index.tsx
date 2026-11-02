/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useState, useEffect, useCallback, useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";

import { SectionPageLayout } from "@/components/layout";
import { useStatus } from "@/hooks/use-status";
import { useSystemConfig } from "@/hooks/use-system-config";
import { getSelf } from "@/lib/api";
import { BillingHistory } from "./components/dialogs/billing-history-dialog";
import { PaymentConfirmDialog } from "./components/dialogs/payment-confirm-dialog";
import { RechargeFormCard } from "./components/recharge-form-card";
import { WalletStatsCard } from "./components/wallet-stats-card";
import { SepayTopup } from "./components/sepay-topup";
import { BinanceUsdtTopup } from "./components/binance-usdt-topup";
import { DEFAULT_DISCOUNT_RATE } from "./constants";
import { useTopupInfo, usePayment } from "./hooks";
import { getDefaultPaymentType, getMinTopupAmount } from "./lib";
import type { UserWalletData, PaymentMethod, PresetAmount } from "./types";

interface WalletProps {
  initialShowHistory?: boolean;
}

export function Wallet(props: WalletProps) {
  const { t } = useTranslation();
  const [user, setUser] = useState<UserWalletData | null>(null);
  const [userLoading, setUserLoading] = useState(true);
  const [topupAmount, setTopupAmount] = useState(0);
  const [selectedPreset, setSelectedPreset] = useState<number | null>(null);
  const [selectedPaymentMethod, setSelectedPaymentMethod] = useState<PaymentMethod>();
  const [paymentLoading, setPaymentLoading] = useState<string | null>(null);
  const [confirmDialogOpen, setConfirmDialogOpen] = useState(false);
  const [historyRefreshKey, setHistoryRefreshKey] = useState(0);
  const [topupTab, setTopupTab] = useState<"sepay" | "binance">("sepay");
  const showHistory = () =>
    document
      .getElementById("wallet-history")
      ?.scrollIntoView({ behavior: "smooth", block: "start" });

  const { status } = useStatus();
  const { currency } = useSystemConfig();
  const { topupInfo, presetAmounts, loading: topupLoading } = useTopupInfo();

  // Calculate effective exchange rate - when display type is USD, use rate of 1
  const effectiveUsdExchangeRate = useMemo(() => {
    return currency?.quotaDisplayType === "USD" ? 1 : currency?.usdExchangeRate || 1;
  }, [currency?.quotaDisplayType, currency?.usdExchangeRate]);
  const {
    amount: paymentAmount,
    calculating,
    processing,
    calculatePaymentAmount,
    processPayment,
  } = usePayment();

  // Fetch and refresh user data
  const fetchUser = useCallback(async () => {
    try {
      setUserLoading(true);
      const response = await getSelf();
      if (response.success && response.data) {
        setUser(response.data as UserWalletData);
      }
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error("Failed to fetch user data:", error);
    } finally {
      setUserLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchUser();
  }, [fetchUser]);

  useEffect(() => {
    if (props.initialShowHistory) {
      showHistory();
    }
  }, [props.initialShowHistory]);

  // Initialize topup amount when topup info is loaded
  const topupAmountInitializedRef = useRef(false);
  useEffect(() => {
    if (topupInfo && !topupAmountInitializedRef.current) {
      topupAmountInitializedRef.current = true;
      const minTopup = getMinTopupAmount(topupInfo);
      setTopupAmount(minTopup);

      // Calculate initial payment amount with default payment type
      const defaultPaymentType = getDefaultPaymentType(topupInfo);
      calculatePaymentAmount(minTopup, defaultPaymentType);
    }
  }, [topupInfo, calculatePaymentAmount]);

  // Get current payment type (selected or default)
  const getCurrentPaymentType = useCallback(() => {
    return selectedPaymentMethod?.type || getDefaultPaymentType(topupInfo);
  }, [selectedPaymentMethod, topupInfo]);

  // Handle preset selection
  const handleSelectPreset = (preset: PresetAmount) => {
    setTopupAmount(preset.value);
    setSelectedPreset(preset.value);
    calculatePaymentAmount(preset.value, getCurrentPaymentType());
  };

  // Handle topup amount change
  const handleTopupAmountChange = (amount: number) => {
    setTopupAmount(amount);
    setSelectedPreset(null);
    calculatePaymentAmount(amount, getCurrentPaymentType());
  };

  // Handle payment method selection
  const handlePaymentMethodSelect = async (method: PaymentMethod) => {
    setSelectedPaymentMethod(method);
    setPaymentLoading(method.type);

    try {
      // Validate minimum topup
      const minTopup = getMinTopupAmount(topupInfo);
      if (topupAmount < minTopup) {
        return;
      }

      // Calculate payment amount and show confirmation dialog
      await calculatePaymentAmount(topupAmount, method.type);
      setConfirmDialogOpen(true);
    } finally {
      setPaymentLoading(null);
    }
  };

  // Handle payment confirmation
  const handlePaymentConfirm = async () => {
    if (!selectedPaymentMethod) return;

    const success = await processPayment(topupAmount, selectedPaymentMethod.type);

    if (success) {
      setConfirmDialogOpen(false);
      setHistoryRefreshKey((key) => key + 1);
      await fetchUser();
    }
  };

  // Get discount rate for current topup amount
  const getDiscountRate = useCallback(() => {
    return topupInfo?.discount?.[topupAmount] || DEFAULT_DISCOUNT_RATE;
  }, [topupInfo, topupAmount]);

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t("Wallet")}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <div className="mx-auto flex w-full max-w-7xl flex-col gap-4 sm:gap-5">
            <WalletStatsCard user={user} loading={userLoading} />
            {user?.id ? (
              <div id="wallet-add-funds" className="scroll-mt-4 space-y-4">
                <div className="flex w-fit rounded-xl border bg-card p-1">
                  <button
                    type="button"
                    className={`rounded-lg px-5 py-2 text-sm font-semibold ${topupTab === "sepay" ? "bg-primary text-primary-foreground" : "text-muted-foreground"}`}
                    onClick={() => setTopupTab("sepay")}
                  >
                    {t("Bank QR")}
                  </button>
                  {topupInfo?.enable_binance_topup ? (
                    <button
                      type="button"
                      className={`rounded-lg px-5 py-2 text-sm font-semibold ${topupTab === "binance" ? "bg-primary text-primary-foreground" : "text-muted-foreground"}`}
                      onClick={() => setTopupTab("binance")}
                    >
                      USDT
                    </button>
                  ) : null}
                </div>
                {topupTab === "sepay" ? (
                  <SepayTopup
                    userId={user.id}
                    onPaid={() => {
                      void fetchUser();
                      setHistoryRefreshKey((key) => key + 1);
                    }}
                  />
                ) : (
                  <BinanceUsdtTopup
                    onPaid={() => {
                      void fetchUser();
                      setHistoryRefreshKey((key) => key + 1);
                    }}
                  />
                )}
              </div>
            ) : null}

            {(topupInfo?.enable_online_topup || topupInfo?.enable_stripe_topup) && (
              <div className="grid gap-4">
                <div>
                  <RechargeFormCard
                    topupInfo={topupInfo}
                    presetAmounts={presetAmounts}
                    selectedPreset={selectedPreset}
                    onSelectPreset={handleSelectPreset}
                    topupAmount={topupAmount}
                    onTopupAmountChange={handleTopupAmountChange}
                    paymentAmount={paymentAmount}
                    calculating={calculating}
                    onPaymentMethodSelect={handlePaymentMethodSelect}
                    paymentLoading={paymentLoading}
                    loading={topupLoading}
                    priceRatio={(status?.price as number) || 1}
                    usdExchangeRate={effectiveUsdExchangeRate}
                    onOpenBilling={showHistory}
                  />
                </div>
              </div>
            )}
            <BillingHistory refreshKey={historyRefreshKey} />
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <PaymentConfirmDialog
        open={confirmDialogOpen}
        onOpenChange={setConfirmDialogOpen}
        onConfirm={handlePaymentConfirm}
        topupAmount={topupAmount}
        paymentAmount={paymentAmount}
        paymentMethod={selectedPaymentMethod}
        calculating={calculating}
        processing={processing}
        discountRate={getDiscountRate()}
        usdExchangeRate={effectiveUsdExchangeRate}
      />
    </>
  );
}
