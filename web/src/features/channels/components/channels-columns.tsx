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
/* eslint-disable react-refresh/only-export-components */
import { useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import {
  AlertTriangle,
  ChevronDown,
  ChevronRight,
  ListOrdered,
  Shuffle,
  SlidersHorizontal,
} from "lucide-react";
import { useState, useMemo, useEffect } from "react";
import { useTranslation } from "react-i18next";

import { ConfirmDialog } from "@/components/confirm-dialog";
import { BadgeListCell } from "@/components/data-table";
import { GroupBadge } from "@/components/group-badge";
import { ProviderBadge } from "@/components/provider-badge";
import { StatusBadge } from "@/components/status-badge";
import { TableId } from "@/components/table-id";
import { TruncatedText } from "@/components/truncated-text";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { toIntlLocale } from "@/i18n/languages";
import { formatTimestampToDate } from "@/lib/format";
import { truncateText } from "@/lib/utils";

import { CHANNEL_STATUS_CONFIG, MODEL_FETCHABLE_TYPES } from "../constants";
import {
  formatRelativeTime,
  formatResponseTime,
  getChannelTypeIcon,
  getChannelTypeLabel,
  getResponseTimeConfig,
  isMultiKeyChannel,
  parseModelsList,
  parseGroupsList,
  parseChannelSettings,
  handleUpdateChannelField,
  handleUpdateTagField,
  createChannelFieldUpdateScheduler,
  isTagAggregateRow,
  type TagRow,
} from "../lib";
import { parseUpstreamUpdateMeta } from "../lib/upstream-update-utils";
import type { Channel } from "../types";
import { useChannels } from "./channels-provider";
import { DataTableRowActions } from "./data-table-row-actions";
import { DataTableTagRowActions } from "./data-table-tag-row-actions";
import { NumericSpinnerInput } from "./numeric-spinner-input";

function parseIonetMeta(otherInfo: string | null | undefined): null | {
  source?: string;
  deployment_id?: string;
} {
  if (!otherInfo) {
    return null;
  }
  try {
    const parsed = JSON.parse(otherInfo);
    if (parsed && typeof parsed === "object") {
      return parsed;
    }
  } catch {
    return null;
  }
  return null;
}

/**
 * Upstream update tags (+N / -N) shown on channel name for model-fetchable channels
 */
function UpstreamUpdateTags({ channel }: { channel: Channel }) {
  const { upstream, setCurrentRow } = useChannels();
  if (!MODEL_FETCHABLE_TYPES.has(channel.type)) {
    return null;
  }

  const meta = parseUpstreamUpdateMeta(channel.settings);
  if (!meta.enabled) {
    return null;
  }

  const addCount = meta.pendingAddModels.length;
  const removeCount = meta.pendingRemoveModels.length;
  if (addCount === 0 && removeCount === 0) {
    return null;
  }

  return (
    <div className="flex items-center gap-0.5">
      {addCount > 0 && (
        <StatusBadge
          label={`+${addCount}`}
          variant="success"
          size="sm"
          copyable={false}
          className="cursor-pointer"
          onClick={(e: React.MouseEvent) => {
            e.stopPropagation();
            setCurrentRow(channel);
            upstream.openModal(channel, meta.pendingAddModels, meta.pendingRemoveModels, "add");
          }}
        />
      )}
      {removeCount > 0 && (
        <StatusBadge
          label={`-${removeCount}`}
          variant="danger"
          size="sm"
          copyable={false}
          className="cursor-pointer"
          onClick={(e: React.MouseEvent) => {
            e.stopPropagation();
            setCurrentRow(channel);
            upstream.openModal(channel, meta.pendingAddModels, meta.pendingRemoveModels, "remove");
          }}
        />
      )}
    </div>
  );
}

/**
 * Priority cell component with inline editing
 */
function PriorityCell({ channel }: { channel: Channel }) {
  if (isTagAggregateRow(channel)) {
    return <TagPriorityCell channel={channel} />;
  }

  return (
    <ChannelFieldCell channelId={channel.id} value={channel.priority} field="priority" min={-999} />
  );
}

function TagPriorityCell({ channel }: { channel: TagRow }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const priority = channel.priority;
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [pendingValue, setPendingValue] = useState<number | null>(null);
  const tag = channel.tag || "";
  const channelCount = channel.children?.length || 0;

  return (
    <>
      <NumericSpinnerInput
        value={priority ?? 0}
        onChange={(value) => {
          setPendingValue(value);
          setConfirmOpen(true);
        }}
        min={-999}
      />
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t("Confirm Batch Update")}
        desc={t(
          'This will update the priority to {{value}} for all {{count}} channel(s) with tag "{{tag}}". Continue?',
          { value: pendingValue, count: channelCount, tag },
        )}
        confirmText={t("Update")}
        handleConfirm={() => {
          if (pendingValue !== null) {
            handleUpdateTagField(tag, "priority", pendingValue, queryClient);
          }
          setConfirmOpen(false);
        }}
      />
    </>
  );
}

function ChannelFieldCell({
  channelId,
  value,
  field,
  min,
}: {
  channelId: number;
  value: number | null | undefined;
  field: "priority" | "weight";
  min: number;
}) {
  const queryClient = useQueryClient();
  const fieldUpdateScheduler = useMemo(
    () =>
      createChannelFieldUpdateScheduler((nextValue) => {
        void handleUpdateChannelField(channelId, field, nextValue, queryClient);
      }),
    [channelId, field, queryClient],
  );

  useEffect(() => () => fieldUpdateScheduler.flush(), [fieldUpdateScheduler]);

  return (
    <NumericSpinnerInput
      value={value ?? 0}
      onChange={fieldUpdateScheduler.schedule}
      onCommit={fieldUpdateScheduler.flush}
      min={min}
    />
  );
}

/**
 * Weight cell component with inline editing
 */
function WeightCell({ channel }: { channel: Channel }) {
  if (isTagAggregateRow(channel)) {
    return <TagWeightCell channel={channel} />;
  }

  return <ChannelFieldCell channelId={channel.id} value={channel.weight} field="weight" min={0} />;
}

function TagWeightCell({ channel }: { channel: TagRow }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const weight = channel.weight;
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [pendingValue, setPendingValue] = useState<number | null>(null);
  const tag = channel.tag || "";
  const channelCount = channel.children?.length || 0;

  return (
    <>
      <NumericSpinnerInput
        value={weight ?? 0}
        onChange={(value) => {
          setPendingValue(value);
          setConfirmOpen(true);
        }}
        min={0}
      />
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t("Confirm Batch Update")}
        desc={t(
          'This will update the weight to {{value}} for all {{count}} channel(s) with tag "{{tag}}". Continue?',
          { value: pendingValue, count: channelCount, tag },
        )}
        confirmText={t("Update")}
        handleConfirm={() => {
          if (pendingValue !== null) {
            handleUpdateTagField(tag, "weight", pendingValue, queryClient);
          }
          setConfirmOpen(false);
        }}
      />
    </>
  );
}

const SENSITIVE_MASK = "••••";

function formatCompactTokens(value: number): string {
  const abs = Math.abs(value);
  if (abs >= 1_000_000_000) {
    const formatted = (value / 1_000_000_000).toFixed(2).replace(/\.?0+$/, "");
    return `${formatted}B`;
  }
  if (abs >= 1_000_000) {
    const formatted = (value / 1_000_000).toFixed(2).replace(/\.?0+$/, "");
    return `${formatted}M`;
  }
  if (abs >= 1_000) {
    const formatted = (value / 1_000).toFixed(1).replace(/\.?0+$/, "");
    return `${formatted}k`;
  }
  return value.toLocaleString();
}

/**
 * Cached usage is display-only. Live quota checks belong to Quota Check.
 */
export function BalanceCell({ channel }: { channel: Channel }) {
  const { t } = useTranslation();
  const { sensitiveVisible } = useChannels();
  if (!sensitiveVisible) return <span>{SENSITIVE_MASK}</span>;
  const used = formatCompactTokens(channel.used_quota || 0);
  const remaining = (channel.balance || 0) >= 99999999
    ? t("Unlimited")
    : formatCompactTokens(Math.round((channel.balance || 0) * 500000));
  return (
    <span className="text-sm tabular-nums" title={t("Used / Remaining")}>
      {used}{!isTagAggregateRow(channel) && <span className="text-muted-foreground"> / {remaining}</span>}
    </span>
  );
}

/**
 * Generate channels columns configuration
 */
export function useChannelsColumns(
  options: {
    enableSelection?: boolean;
  } = {},
): ColumnDef<Channel>[] {
  const { t, i18n } = useTranslation();
  const { sensitiveVisible } = useChannels();
  const enableSelection = options.enableSelection ?? true;
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language);
  // The column definitions only depend on the translation function, the active
  // locale, and sensitive-data visibility. Memoizing keeps the array (and every
  // cell renderer reference) stable across unrelated re-renders, so react-table
  // does not invalidate the whole row model on each parent render.
  return useMemo<ColumnDef<Channel>[]>(
    () => [
      // Checkbox column
      ...(enableSelection
        ? [
            {
              id: "select",
              header: ({ table }) => (
                <Checkbox
                  checked={table.getIsAllPageRowsSelected()}
                  indeterminate={table.getIsSomePageRowsSelected()}
                  onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
                  aria-label={t("Select all")}
                />
              ),
              cell: ({ row }) => {
                const isTagRow = isTagAggregateRow(row.original);

                // Don't show checkbox for tag rows
                if (isTagRow) {
                  return null;
                }

                return (
                  <Checkbox
                    checked={row.getIsSelected()}
                    onCheckedChange={(value) => row.toggleSelected(!!value)}
                    aria-label={t("Select row")}
                  />
                );
              },
              enableSorting: false,
              enableHiding: false,
              enableResizing: false,
              size: 40,
            } satisfies ColumnDef<Channel>,
          ]
        : []),

      // ID column
      {
        accessorKey: "id",
        header: t("ID"),
        meta: { mobileHidden: true },
        cell: ({ row }) => {
          const id = row.getValue("id") as number;
          return <TableId value={sensitiveVisible ? id : SENSITIVE_MASK} />;
        },
        size: 80,
      },
      // Name column
      {
        accessorKey: "name",
        header: t("Name"),
        meta: { mobileTitle: true },
        cell: ({ row }) => {
          const isTagRow = isTagAggregateRow(row.original);
          const name = row.getValue("name") as string;
          const channel = row.original;

          // Tag row with expand/collapse
          if (isTagRow) {
            const tag = (row.original as TagRow).tag || name;
            const childrenCount = (row.original as TagRow).children?.length || 0;

            return (
              <div className="flex items-center gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-6 w-6 p-0"
                  onClick={row.getToggleExpandedHandler()}
                >
                  {row.getIsExpanded() ? (
                    <ChevronDown className="h-4 w-4" />
                  ) : (
                    <ChevronRight className="h-4 w-4" />
                  )}
                </Button>
                <div className="flex items-center gap-1.5">
                  <span className="font-semibold">Tag：{tag}</span>
                  <StatusBadge
                    label={`${childrenCount} channels`}
                    variant="blue"
                    size="sm"
                    copyable={false}
                  />
                </div>
              </div>
            );
          }

          // Regular channel row
          const settings = parseChannelSettings(channel.setting);
          const isPassThrough = settings.pass_through_body_enabled === true;
          const hasParamOverride = Boolean(channel.param_override?.trim());

          return (
            <div className="flex max-w-full min-w-0 items-center gap-2">
              <div className="flex max-w-full min-w-0 flex-col gap-1">
                <div className="flex max-w-full min-w-0 items-center gap-1.5">
                  <TruncatedText
                    text={sensitiveVisible ? name : SENSITIVE_MASK}
                    className="font-medium"
                    maxWidth="max-w-full"
                  />
                  {isPassThrough && (
                    <TooltipProvider delay={100}>
                      <Tooltip>
                        <TooltipTrigger
                          render={
                            <AlertTriangle className="h-3.5 w-3.5 flex-shrink-0 text-amber-500" />
                          }
                        />
                        <TooltipContent side="top">
                          {t(
                            "Request body pass-through is enabled. The request body will be sent directly to the upstream without any conversion.",
                          )}
                        </TooltipContent>
                      </Tooltip>
                    </TooltipProvider>
                  )}
                  {hasParamOverride && (
                    <TooltipProvider delay={100}>
                      <Tooltip>
                        <TooltipTrigger
                          render={
                            <SlidersHorizontal className="text-info h-3.5 w-3.5 flex-shrink-0" />
                          }
                        />
                        <TooltipContent side="top">
                          {t("Override request parameters")}
                        </TooltipContent>
                      </Tooltip>
                    </TooltipProvider>
                  )}
                  <UpstreamUpdateTags channel={channel} />
                </div>
                {channel.remark && (
                  <TooltipProvider delay={200}>
                    <Tooltip>
                      <TooltipTrigger render={<span className="text-muted-foreground text-xs" />}>
                        {truncateText(channel.remark, 40)}
                      </TooltipTrigger>
                      <TooltipContent side="bottom" className="max-w-xs">
                        {channel.remark}
                      </TooltipContent>
                    </Tooltip>
                  </TooltipProvider>
                )}
              </div>
            </div>
          );
        },
        size: 260,
        minSize: 200,
      },

      // Type column
      {
        accessorKey: "type",
        header: t("Type"),
        cell: ({ row }) => {
          const isTagRow = isTagAggregateRow(row.original);

          if (isTagRow) {
            return (
              <StatusBadge
                label={t("Tag Aggregate")}
                variant="blue"
                size="sm"
                copyable={false}
                className="-ml-1.5"
              />
            );
          }

          const type = row.getValue("type") as number;
          const typeNameKey = getChannelTypeLabel(type);
          const typeName = t(typeNameKey);
          const iconName = getChannelTypeIcon(type);
          const channel = row.original as Channel;
          const isMultiKey = isMultiKeyChannel(channel);
          const multiKeyMode = channel.channel_info?.multi_key_mode ?? "random";
          const MultiKeyModeIcon = multiKeyMode === "random" ? Shuffle : ListOrdered;
          const multiKeyTooltip =
            multiKeyMode === "random"
              ? t("Multi-key: Random rotation")
              : t("Multi-key: Polling rotation");

          const ionetMeta = parseIonetMeta(channel.other_info);
          const isIonet = ionetMeta?.source === "ionet";
          const deploymentId =
            typeof ionetMeta?.deployment_id === "string" ? ionetMeta?.deployment_id : undefined;

          return (
            <div className="flex max-w-full min-w-0 items-center gap-2 overflow-hidden">
              {isMultiKey && (
                <TooltipProvider delay={100}>
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <span className="border-border bg-muted text-primary inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-md border" />
                      }
                    >
                      <MultiKeyModeIcon className="h-3 w-3" />
                    </TooltipTrigger>
                    <TooltipContent side="top">{multiKeyTooltip}</TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              )}
              <TooltipProvider delay={300}>
                <Tooltip>
                  <TooltipTrigger render={<div className="max-w-full min-w-0 overflow-hidden" />}>
                    <ProviderBadge
                      iconKey={`${iconName}.Color`}
                      iconSize={18}
                      label={typeName}
                      colorText={false}
                      copyable={false}
                      showDot={false}
                      className="max-w-full min-w-0 overflow-hidden"
                    />
                  </TooltipTrigger>
                  <TooltipContent side="top">{typeName}</TooltipContent>
                </Tooltip>
              </TooltipProvider>
              {isIonet && (
                <TooltipProvider delay={100}>
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <span
                          className="flex cursor-pointer items-center gap-1.5 text-xs font-medium"
                          onClick={(e) => {
                            e.stopPropagation();
                            if (!deploymentId) {
                              return;
                            }
                            const targetUrl = `/models/deployments?dFilter=${encodeURIComponent(String(deploymentId))}`;
                            window.open(targetUrl, "_blank", "noopener");
                          }}
                        />
                      }
                    >
                      <StatusBadge
                        label="IO.NET"
                        variant="purple"
                        size="sm"
                        copyable={false}
                        className="cursor-pointer"
                      />
                    </TooltipTrigger>
                    <TooltipContent side="top">
                      <div className="max-w-xs space-y-1">
                        <div className="text-xs">{t("From IO.NET deployment")}</div>
                        {deploymentId && (
                          <div className="text-muted-foreground font-mono text-xs">
                            {t("Deployment ID")}: {deploymentId}
                          </div>
                        )}
                        <div className="text-muted-foreground text-xs">
                          {t("Click to open deployment")}
                        </div>
                      </div>
                    </TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              )}
            </div>
          );
        },
        filterFn: (row, id, value) => {
          if (!value || value.length === 0 || value.includes("all")) {
            return true;
          }
          return value.includes(String(row.getValue(id)));
        },
        size: 220,
        enableSorting: false,
      },

      // Status column
      {
        accessorKey: "status",
        header: t("Status"),
        meta: { mobileBadge: true },
        cell: ({ row }) => {
          const isTagRow = isTagAggregateRow(row.original);
          const status = row.getValue("status") as number;
          const channel = row.original as Channel;

          // Tag row: show aggregated status
          if (isTagRow) {
            const childrenCount = (row.original as TagRow).children?.length || 0;
            const hasEnabled = status === 1;

            if (hasEnabled) {
              return (
                <StatusBadge
                  label={`Active (${childrenCount})`}
                  variant="success"
                  size="sm"
                  copyable={false}
                  className="-ml-1.5"
                />
              );
            } else {
              return (
                <StatusBadge
                  label={`Inactive (${childrenCount})`}
                  variant="neutral"
                  size="sm"
                  copyable={false}
                  className="-ml-1.5"
                />
              );
            }
          }

          // Regular channel row
          const config =
            CHANNEL_STATUS_CONFIG[status as keyof typeof CHANNEL_STATUS_CONFIG] ||
            CHANNEL_STATUS_CONFIG[0];

          const isMultiKey = isMultiKeyChannel(channel);
          const keySize = channel.channel_info?.multi_key_size ?? 0;
          const disabledCount = channel.channel_info?.multi_key_status_list
            ? Object.keys(channel.channel_info.multi_key_status_list).length
            : 0;
          const enabledCount = Math.max(0, keySize - disabledCount);
          const label =
            isMultiKey && keySize > 0
              ? `${t(config.label)} (${enabledCount}/${keySize})`
              : t(config.label);

          // Auto-disabled: show reason and time tooltip
          if (status === 3) {
            let statusReason = "";
            let statusTime = "";
            try {
              const otherInfo = channel.other_info ? JSON.parse(channel.other_info) : null;
              if (otherInfo) {
                statusReason = otherInfo.status_reason || "";
                statusTime = otherInfo.status_time
                  ? formatTimestampToDate(otherInfo.status_time)
                  : "";
              }
            } catch {
              /* empty */
            }

            if (statusReason || statusTime) {
              return (
                <TooltipProvider delay={100}>
                  <Tooltip>
                    <TooltipTrigger render={<span />}>
                      <StatusBadge
                        label={label}
                        variant={config.variant}
                        size="sm"
                        copyable={false}
                      />
                    </TooltipTrigger>
                    <TooltipContent side="top" className="max-w-xs">
                      <div className="space-y-1 text-xs">
                        {statusReason && (
                          <div>
                            {t("Reason:")} {statusReason}
                          </div>
                        )}
                        {statusTime && (
                          <div>
                            {t("Time:")} {statusTime}
                          </div>
                        )}
                      </div>
                    </TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              );
            }
          }

          return <StatusBadge label={label} variant={config.variant} size="sm" copyable={false} />;
        },
        filterFn: (row, id, value) => {
          if (!value || value.length === 0 || value.includes("all")) {
            return true;
          }
          const status = row.getValue(id) as number;
          if (value.includes("enabled")) {
            return status === 1;
          }
          if (value.includes("disabled")) {
            return status !== 1;
          }
          return false;
        },
        size: 120,
        enableSorting: false,
      },

      // Models column
      {
        accessorKey: "models",
        header: t("Models"),
        meta: { mobileHidden: true },
        cell: ({ row }) => {
          const models = row.getValue("models") as string;
          const modelArray = parseModelsList(models);
          return (
            <BadgeListCell
              items={modelArray.map((model) => (
                <StatusBadge
                  key={model}
                  label={model}
                  autoColor={model}
                  size="sm"
                  className="font-mono"
                />
              ))}
            />
          );
        },
        size: 200,
        enableSorting: false,
      },

      // Group column
      {
        accessorKey: "group",
        header: t("Groups"),
        meta: { mobileHidden: true },
        cell: ({ row }) => {
          const group = row.getValue("group") as string;
          const groupArray = parseGroupsList(group);
          return (
            <BadgeListCell
              items={groupArray.map((g) => (
                <GroupBadge
                  key={g}
                  group={g}
                  label={sensitiveVisible ? undefined : SENSITIVE_MASK}
                  size="sm"
                />
              ))}
            />
          );
        },
        filterFn: (row, id, value) => {
          if (!value || value.length === 0 || value.includes("all")) {
            return true;
          }
          const group = row.getValue(id) as string;
          const groupArray = parseGroupsList(group);
          return groupArray.some((g) => value.includes(g));
        },
        size: 150,
        enableSorting: false,
      },

      // Tag column
      {
        accessorKey: "tag",
        header: t("Tag"),
        meta: { mobileHidden: true },
        cell: ({ row }) => {
          const tag = row.getValue("tag") as string | null;
          if (!tag) {
            return <span className="text-muted-foreground text-xs">-</span>;
          }

          return <StatusBadge label={tag} autoColor={tag} size="sm" className="-ml-1.5" />;
        },
        size: 120,
        enableSorting: false,
      },

      // Priority column
      {
        accessorKey: "priority",
        header: t("Priority"),
        meta: { mobileHidden: true },
        cell: ({ row }) => <PriorityCell channel={row.original} />,
        size: 100,
      },

      // Weight column
      {
        accessorKey: "weight",
        header: t("Weight"),
        meta: { mobileHidden: true },
        cell: ({ row }) => <WeightCell channel={row.original} />,
        size: 90,
        enableSorting: false,
      },

      // Balance column (Used/Remaining)
      {
        accessorKey: "balance",
        header: t("Used / Remaining"),
        cell: ({ row }) => <BalanceCell channel={row.original} />,
        size: 180,
      },

      // Response Time column
      {
        accessorKey: "response_time",
        header: t("Response"),
        meta: { mobileHidden: true },
        cell: ({ row }) => {
          const responseTime = row.getValue("response_time") as number;
          const config = getResponseTimeConfig(responseTime);

          return (
            <StatusBadge
              label={formatResponseTime(responseTime, t)}
              variant={config.variant}
              size="sm"
              copyable={false}
              className="-ml-1.5"
            />
          );
        },
        size: 110,
      },

      // Test Time column
      {
        accessorKey: "test_time",
        header: t("Last Tested"),
        meta: { mobileHidden: true },
        cell: ({ row }) => {
          const testTime = row.getValue("test_time") as number;

          // For invalid timestamps, show "Never" badge
          if (!testTime || testTime === 0) {
            return <span className="text-muted-foreground text-xs">-</span>;
          }

          const timeText = formatRelativeTime(testTime, locale);
          const fullDate = formatTimestampToDate(testTime);

          // For valid timestamps, show tooltip with full date
          return (
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <StatusBadge
                      label={timeText}
                      variant="neutral"
                      size="sm"
                      copyable={false}
                      className="-ml-1.5 cursor-pointer"
                    />
                  }
                />
                <TooltipContent side="top">
                  <p className="font-mono text-sm">{fullDate}</p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          );
        },
        size: 120,
        enableSorting: false,
      },

      // Actions column
      {
        id: "actions",
        header: () => t("Actions"),
        cell: ({ row }) => {
          // Check if this is a tag row (has children)
          const isTagRow = isTagAggregateRow(row.original);

          if (isTagRow) {
            return (
              <DataTableTagRowActions
                // eslint-disable-next-line @typescript-eslint/no-explicit-any
                row={row as any}
              />
            );
          }

          return <DataTableRowActions row={row} />;
        },
        enableSorting: false,
        enableHiding: false,
        meta: { pinned: "right" as const },
      },
    ],
    [enableSelection, t, locale, sensitiveVisible],
  );
}
