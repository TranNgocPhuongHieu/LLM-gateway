import { CheckCircle2, QrCode, Sparkles } from "lucide-react";
import type { ReactNode } from "react";

// Shared presentation for the bank QR and USDT top-up tabs so both read as one
// checkout: same preset cards, receive summary, next-tier hint and empty state.
// Amounts are computed by the callers; these components only render them.

export type TopupPreset = {
  value: string;
  label: string;
  bonusCredits: number;
  popular?: boolean;
};

export function PresetGrid({
  presets,
  selected,
  disabled,
  onSelect,
  format,
  vi,
}: {
  presets: TopupPreset[];
  selected: string;
  disabled?: boolean;
  onSelect: (value: string) => void;
  format: (n: number) => string;
  vi: boolean;
}) {
  return (
    <div className="my-6 grid grid-cols-2 gap-3 sm:grid-cols-[repeat(auto-fit,minmax(7.5rem,1fr))]">
      {presets.map((preset) => (
        <button
          type="button"
          key={preset.value}
          aria-pressed={selected === preset.value}
          disabled={disabled}
          onClick={() => onSelect(preset.value)}
          className={`relative rounded-xl border px-2 py-4 text-center transition-colors disabled:opacity-60 ${selected === preset.value ? "border-primary bg-primary/10 ring-1 ring-primary" : "bg-muted/30 hover:border-primary/50"}`}
        >
          {preset.popular && (
            <span className="absolute -top-2 right-2 rounded-full bg-primary px-2 text-[10px] text-primary-foreground">
              {vi ? "Phổ biến" : "Popular"}
            </span>
          )}
          <span className="block font-bold">{preset.label}</span>
          {preset.bonusCredits > 0 && (
            <span className="mt-1 block text-xs font-medium text-emerald-500">
              +{format(preset.bonusCredits)} credit {vi ? "thưởng" : "bonus"}
            </span>
          )}
        </button>
      ))}
    </div>
  );
}

export function ReceiveSummary({
  baseCredits,
  bonusCredits,
  bonusPercent,
  format,
  vi,
  aside,
}: {
  baseCredits: number | null;
  bonusCredits: number;
  bonusPercent: number;
  format: (n: number) => string;
  vi: boolean;
  aside?: ReactNode;
}) {
  const total = baseCredits === null ? null : baseCredits + bonusCredits;
  return (
    <div className="my-6 flex items-end justify-between gap-4 rounded-xl border border-emerald-500/20 bg-emerald-500/5 p-4">
      <div>
        <p className="text-xs text-muted-foreground">{vi ? "Bạn nhận" : "You receive"}</p>
        <p className="mt-1 text-3xl font-bold text-emerald-500" data-testid="receive-total">
          {total === null ? "—" : format(total)}{" "}
          <span className="text-base font-semibold">credit</span>
        </p>
        {total !== null && bonusCredits > 0 && (
          <p className="mt-1 text-xs text-muted-foreground" data-testid="receive-breakdown">
            {format(baseCredits!)} + {format(bonusCredits)} {vi ? "thưởng" : "bonus"} (+
            {format(bonusPercent)}%)
          </p>
        )}
      </div>
      {aside && <div className="text-right">{aside}</div>}
    </div>
  );
}

// Tier thresholds must be sorted ascending. Returns null when the amount is
// already in the highest bonus tier or above the allowed maximum.
export function nextTier<T extends { min: number; percent: number }>(
  amount: number,
  currentPercent: number,
  tiers: T[],
  max?: number,
) {
  const next = tiers.find((tier) => tier.min > amount && tier.percent > currentPercent);
  if (!next || (max !== undefined && next.min > max)) return null;
  return next;
}

export function NextTierHint({ children }: { children: ReactNode }) {
  return (
    <p
      className="-mt-3 mb-4 flex items-center gap-2 rounded-lg bg-amber-500/10 px-3 py-2 text-sm text-amber-600"
      data-testid="next-tier-hint"
    >
      <Sparkles className="size-4 shrink-0" />
      <span>{children}</span>
    </p>
  );
}

export function TopupSteps({
  title,
  steps,
  footer,
}: {
  title: string;
  steps: string[];
  footer: string;
}) {
  return (
    <div className="flex min-h-72 flex-col justify-center rounded-xl border border-dashed p-5 sm:p-7">
      <QrCode className="mx-auto mb-4 size-14 text-muted-foreground" />
      <h3 className="text-center font-bold">{title}</h3>
      <ol className="mx-auto mt-5 w-full max-w-sm space-y-3">
        {steps.map((step, index) => (
          <li key={step} className="flex items-start gap-3 text-sm">
            <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary/15 text-xs font-semibold text-primary">
              {index + 1}
            </span>
            <span className="pt-0.5 text-muted-foreground">{step}</span>
          </li>
        ))}
      </ol>
      <p className="mx-auto mt-6 flex max-w-sm items-start gap-2 text-xs text-muted-foreground">
        <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-emerald-500" />
        {footer}
      </p>
    </div>
  );
}
