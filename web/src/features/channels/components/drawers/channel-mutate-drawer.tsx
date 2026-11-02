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
import { zodResolver } from "@hookform/resolvers/zod";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  ChevronDown,
  ClipboardPaste,
  Loader2,
  Server,
  FileText,
  Route,
  Settings,
} from "lucide-react";
import {
  type ComponentProps,
  type ReactNode,
  useEffect,
  useState,
  useMemo,
  useCallback,
  useRef,
} from "react";
import { type SubmitErrorHandler, useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import {
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from "@/components/drawer-layout";
import { ErrorState } from "@/components/error-state";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { IconBadge, type IconBadgeTone } from "@/components/ui/icon-badge";
import { Input } from "@/components/ui/input";
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { SecureVerificationDialog } from "@/features/auth/secure-verification";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from "@/lib/admin-permissions";
import {
  parseChannelConnectionInfo,
  type ChannelConnectionInfo,
} from "@/lib/channel-connection-info";
import { handleServerError } from "@/lib/handle-server-error";
import { ROLE } from "@/lib/roles";
import { requireServerSuccess, getServerErrorMessage } from "@/lib/server-error-message";
import { cn } from "@/lib/utils";
import { useAuthStore } from "@/stores/auth-store";

import { getAllModels, getChannel, getGroups, getPrefillGroups } from "../../api";
import {
  ADD_MODE_OPTIONS,
  CHANNEL_STATUS_LABELS,
  CHANNEL_TYPE_OPTIONS,
  ERROR_MESSAGES,
  FIELD_DESCRIPTIONS,
  FIELD_PLACEHOLDERS,
  MODEL_FETCHABLE_TYPES,
} from "../../constants";
import { useChannelKeyDisclosure } from "../../hooks/use-channel-key-disclosure";
import {
  useChannelModelDiscovery,
  type ChannelModelDiscoveryRequest,
} from "../../hooks/use-channel-model-discovery";
import { useChannelMutateForm } from "../../hooks/use-channel-mutate-form";
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  channelsQueryKeys,
  transformChannelToFormDefaults,
  type ChannelFormValues,
  deduplicateKeys,
  parseModelsString,
  formatModelsArray,
  extractRedirectModels,
  extractMappingSourceModels,
  hasModelConfigChanged,
  findMissingModelsInMapping,
  validateModelMappingJson,
} from "../../lib";
import {
  getChannelConfigurationSection,
  getChannelConfigurationState,
  type ChannelConfigurationStatus,
  type ChannelConfigurationSection,
  type ChannelProviderTarget,
} from "../../lib/channel-configuration";
import {
  collectInvalidStatusCodeEntries,
  collectNewDisallowedStatusCodeRedirects,
} from "../../lib/status-code-risk-guard";
import type { Channel } from "../../types";
import { ChannelTypeLogo } from "../channel-type-badge";
import { useChannels } from "../channels-provider";
import { ConfigureModelsDialog } from "../dialogs/configure-models-dialog";
import {
  MissingModelsConfirmationDialog,
  type MissingModelsAction,
} from "../dialogs/missing-models-confirmation-dialog";
import { ParamOverrideEditorDialog } from "../dialogs/param-override-editor-dialog";
import { StatusCodeRiskDialog } from "../dialogs/status-code-risk-dialog";
import { getKeyPromptForType } from "../../lib";
import { ChannelConfiguration, ChannelConfigurationStatusIndicator } from "./channel-configuration";
import { ChannelProviderPicker } from "./channel-provider-picker";
import {
  ChannelFormatField,
  ChannelHttpProtocolField,
  ChannelHttpShardsField,
  ChannelPassThroughField,
  ChannelProxyField,
  ChannelSystemPromptField,
  ChannelSystemPromptOverrideField,
  ChannelThinkingField,
} from "./channel-mutate-protocol-sections";
import { buildChannelModelSections } from "./channel-mutate-model-sections";
import { buildChannelConnectionSections } from "./channel-mutate-connection-sections";
import { ChannelEditorLoadingState } from "./sections";

type ChannelMutateDrawerProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  currentRow?: Channel | null;
};

type ModelMappingGuardrail = {
  invalidJson: boolean;
  entries: Array<{ source: string; target: string }>;
  missingSourceModels: string[];
  exposedTargetModels: string[];
};

// Helper functions
const createEmptyModelMappingGuardrail = (): ModelMappingGuardrail => ({
  invalidJson: false,
  entries: [],
  missingSourceModels: [],
  exposedTargetModels: [],
});

const formatModelNames = (models: string[]): string =>
  models.map((model) => `"${model}"`).join(", ");

const MODEL_MAPPING_PREVIEW_FALLBACK: Array<{
  source: string;
  target: string;
}> = [{ source: "client-model", target: "upstream-model" }];

const UPSTREAM_DETECTED_MODEL_PREVIEW_LIMIT = 8;
const SENSITIVE_FORM_FIELDS = [
  "type",
  "base_url",
  "key",
  "openai_organization",
  "other",
  "key_mode",
  "param_override",
  "header_override",
  "settings",
  "setting",
  "force_format",
  "thinking_to_content",
  "proxy",
  "http_protocol",
  "http2_connection_shards",
  "pass_through_body_enabled",
  "system_prompt",
  "system_prompt_override",
  "allow_service_tier",
  "disable_store",
  "allow_safety_identifier",
  "allow_include_obfuscation",
  "allow_inference_geo",
  "allow_speed",
  "claude_beta_query",
  "upstream_model_update_check_enabled",
  "upstream_model_update_auto_sync_enabled",
  "upstream_model_update_ignored_models",
] satisfies (keyof ChannelFormValues)[];

function parseSettingsRecord(settings: string | undefined): Record<string, unknown> {
  if (!settings?.trim()) return {};
  try {
    const parsed = JSON.parse(settings);
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
  } catch {
    return {};
  }
  return {};
}

function formatUnixTime(timestamp: unknown): string {
  const seconds = Number(timestamp);
  if (!Number.isFinite(seconds) || seconds <= 0) return "-";
  return new Date(seconds * 1000).toLocaleString();
}

function channelConfigurationBlockClassName(
  status: ChannelConfigurationStatus,
  className?: string,
) {
  return cn(
    "border-border/60 rounded-lg border p-4 transition-colors",
    status === "configured" && "border-primary/35 ring-primary/20 ring-1",
    status === "error" && "border-destructive/50 ring-destructive/20 ring-1",
    className,
  );
}

function CardHeading(props: {
  title: string;
  icon?: ReactNode;
  iconTone?: IconBadgeTone;
  status?: ChannelConfigurationStatus;
}) {
  return (
    <div className="flex items-center gap-3">
      {props.icon && (
        <IconBadge tone={props.iconTone} size="md">
          {props.icon}
        </IconBadge>
      )}
      <h3 className="min-w-0 flex-1 text-sm font-semibold tracking-tight">{props.title}</h3>
      {props.status && <ChannelConfigurationStatusIndicator status={props.status} />}
    </div>
  );
}

function SubHeading(props: {
  title: string;
  icon?: ReactNode;
  iconTone?: IconBadgeTone;
  status?: ChannelConfigurationStatus;
}) {
  return (
    <div className="flex items-center gap-2">
      {props.icon && (
        <IconBadge tone={props.iconTone} size="xs">
          {props.icon}
        </IconBadge>
      )}
      <h4 className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
        {props.title}
      </h4>
      {props.status && <ChannelConfigurationStatusIndicator status={props.status} />}
    </div>
  );
}

export function ChannelMutateDrawer({ open, onOpenChange, currentRow }: ChannelMutateDrawerProps) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { setOpen } = useChannels();
  const currentUser = useAuthStore((s) => s.auth.user);
  const canEditSensitive = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE,
  );
  const canOperateChannel = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.OPERATE,
  );
  const canRevealChannelKey = currentUser?.role === ROLE.SUPER_ADMIN;
  const initialModelsRef = useRef<string[]>([]);
  const initialModelMappingRef = useRef<string>("");
  const initialStatusCodeMappingRef = useRef<string>("");
  const [statusCodeRiskOpen, setStatusCodeRiskOpen] = useState(false);
  const [statusCodeRiskDetailItems, setStatusCodeRiskDetailItems] = useState<string[]>([]);
  const statusCodeRiskResolveRef = useRef<((confirmed: boolean) => void) | null>(null);
  const [missingModelsDialogOpen, setMissingModelsDialogOpen] = useState(false);
  const [missingModelsList, setMissingModelsList] = useState<string[]>([]);
  const missingModelsResolveRef = useRef<((action: MissingModelsAction) => void) | null>(null);
  const channelFormRef = useRef<HTMLFormElement>(null);
  const [modelConfiguration, setModelConfiguration] = useState<{
    pluginKey?: string;
  } | null>(null);
  const [paramOverrideEditorOpen, setParamOverrideEditorOpen] = useState(false);
  const [clipboardConnectionInfo, setClipboardConnectionInfo] =
    useState<ChannelConnectionInfo | null>(null);

  const isEditing = Boolean(currentRow);
  const requestedSide = isEditing ? "left" : "right";
  const [drawerSide, setDrawerSide] = useState<"left" | "right">(requestedSide);
  // The parent clears currentRow as soon as closing starts. Keep the last
  // open direction until the next opening, including the entire exit animation.
  if (open && drawerSide !== requestedSide) {
    setDrawerSide(requestedSide);
  }
  const channelId = currentRow?.id ?? null;
  const sensitiveLocked = isEditing && !canEditSensitive;
  const [providerTarget, setProviderTarget] = useState<ChannelProviderTarget | null>(null);
  const [choosingProvider, setChoosingProvider] = useState(true);
  const [configurationSection, setConfigurationSection] =
    useState<ChannelConfigurationSection>("connection");
  const [pendingErrorFocus, setPendingErrorFocus] = useState<string | null>(null);
  const showProviderPicker = choosingProvider && (!isEditing || Boolean(providerTarget));
  const providerControlRef = useRef<HTMLButtonElement>(null);
  const previousProviderPicker = useRef(false);
  useEffect(() => {
    if (open && previousProviderPicker.current && !showProviderPicker) {
      providerControlRef.current?.focus({ preventScroll: true });
    }
    previousProviderPicker.current = showProviderPicker;
  }, [open, showProviderPicker]);
  const loadedForm = useRef<{ channelId: number; snapshot: string } | null>(null);

  // Fetch channel details if editing
  const {
    data: channelData,
    isLoading: isChannelLoading,
    isError: isChannelError,
    error: channelError,
    refetch: refetchChannel,
  } = useQuery({
    queryKey: channelsQueryKeys.detail(channelId || 0),
    queryFn: async () => requireServerSuccess(await getChannel(channelId || 0)),
    enabled: open && isEditing && Boolean(channelId),
    meta: { errorToast: false },
  });

  // Fetch available groups
  const { data: groupsData, isLoading: isLoadingGroups } = useQuery({
    queryKey: ["groups"],
    queryFn: async () => requireServerSuccess(await getGroups()),
    enabled: open && !showProviderPicker,
  });

  // Fetch all available models
  const { data: allModelsData } = useQuery({
    queryKey: ["channel_models"],
    queryFn: async () => requireServerSuccess(await getAllModels()),
    enabled: open && !showProviderPicker,
  });

  // Fetch prefill model groups
  const { data: prefillGroupsData } = useQuery({
    queryKey: ["prefill_groups", "model"],
    queryFn: async () => requireServerSuccess(await getPrefillGroups("model")),
    enabled: open && !showProviderPicker,
  });

  const { copyToClipboard } = useCopyToClipboard();

  const { channelKey, isChannelKeyLoading, handleRevealKey, verification } =
    useChannelKeyDisclosure(open, channelId);

  // Check if this is a multi-key channel
  const isMultiKeyChannel = isEditing && channelData?.data?.channel_info?.is_multi_key === true;

  // Form setup
  const form = useForm<ChannelFormValues>({
    resolver: zodResolver(channelFormSchema),
    defaultValues: CHANNEL_FORM_DEFAULT_VALUES,
  });

  // Watch values once for conditional fields and configuration indicators.
  const formValues = form.watch();
  const multiKeyMode = formValues.multi_key_mode;
  const multiKeyType = formValues.multi_key_type;
  const keyMode = formValues.key_mode;
  const currentGroups = formValues.group;
  const currentType = formValues.type;
  const currentStatus = formValues.status;
  const currentBaseUrl = formValues.base_url;
  const currentKey = formValues.key;
  const currentModels = formValues.models;
  const currentModelMapping = formValues.model_mapping;
  const upstreamModelUpdateCheckEnabled = formValues.upstream_model_update_check_enabled;
  const currentSettings = formValues.settings;
  const currentHeaderOverride = formValues.header_override;
  const currentProxy = formValues.proxy;
  const currentHttpProtocol = formValues.http_protocol;
  const applyConnectionInfo = useCallback(
    (connectionInfo: ChannelConnectionInfo) => {
      form.setValue("key", connectionInfo.key, {
        shouldDirty: true,
        shouldValidate: true,
      });
      form.setValue("base_url", connectionInfo.url, {
        shouldDirty: true,
        shouldValidate: true,
      });
      setClipboardConnectionInfo(null);
      toast.success(t("Connection info filled in"));
    },
    [form, t],
  );

  const pasteConnectionInfoFromClipboard = useCallback(async () => {
    if (typeof navigator === "undefined" || !navigator.clipboard?.readText) {
      toast.error(t("Unable to read clipboard"));
      return;
    }

    try {
      const text = await navigator.clipboard.readText();
      const parsed = parseChannelConnectionInfo(text);
      if (parsed) {
        applyConnectionInfo(parsed);
        return;
      }
      toast.info(t("No connection info found in clipboard"));
    } catch {
      toast.error(t("Unable to read clipboard"));
    }
  }, [applyConnectionInfo, t]);

  useEffect(() => {
    if (!open || isEditing || showProviderPicker) {
      setClipboardConnectionInfo(null);
      return;
    }

    if (typeof navigator === "undefined" || !navigator.clipboard?.readText) {
      return;
    }

    let cancelled = false;
    void navigator.clipboard
      .readText()
      .then((text) => {
        if (cancelled) return;
        setClipboardConnectionInfo(parseChannelConnectionInfo(text));
      })
      .catch(() => {
        /* Clipboard detection is best-effort on drawer open. */
      });

    return () => {
      cancelled = true;
    };
  }, [isEditing, open, showProviderPicker]);

  // Helper computed values
  const isBatchMode = multiKeyMode === "batch" || multiKeyMode === "multi_to_single";
  const isChannelDetailLoading = isEditing && isChannelLoading;
  const addModeOptions = ADD_MODE_OPTIONS;

  // Get all models list
  const allModelsList = useMemo(
    () => allModelsData?.data?.map((model) => model.id).filter(Boolean) || [],
    [allModelsData],
  );

  // Get basic models for the current channel type
  const basicModels = useMemo(() => {
    if (!allModelsList.length) return [];
    // Filter models based on common patterns for specific types
    if (currentType === 1) {
      return allModelsList.filter((model) => model.startsWith("gpt-") || model.startsWith("text-"));
    }
    return allModelsList;
  }, [allModelsList, currentType]);

  // Get prefill groups
  const prefillGroups = useMemo(() => prefillGroupsData?.data || [], [prefillGroupsData]);

  // Transform groups to multi-select options
  const groupOptions = useMemo(() => {
    if (!groupsData?.data) return [];
    const allGroups = new Set([...groupsData.data, ...(currentGroups || [])]);
    return [...allGroups].map((group) => ({
      value: group,
      label: group,
    }));
  }, [groupsData, currentGroups]);

  // Parse current models as array
  const currentModelsArray = useMemo(() => parseModelsString(currentModels), [currentModels]);

  const currentTypeLabel = useMemo(
    () =>
      CHANNEL_TYPE_OPTIONS.find((option) => option.value === currentType)?.label ||
      `#${currentType}`,
    [currentType],
  );
  const providerLabel = t(currentTypeLabel);
  const selectProvider = useCallback(
    (target: ChannelProviderTarget) => {
      if (!canEditSensitive || target.kind !== "builtin") return;
      form.setValue("type", target.type, { shouldDirty: true });
      if (!isEditing && !form.getValues("name").trim()) {
        const label = CHANNEL_TYPE_OPTIONS.find((option) => option.value === target.type)?.label;
        form.setValue("name", t(label || "OpenAI Compatible"));
      }
      setProviderTarget(target);
      setChoosingProvider(false);
    },
    [canEditSensitive, form, isEditing, t],
  );

  const formErrors = form.formState.errors;
  const configuration = getChannelConfigurationState(formValues, formErrors, isEditing);

  // Extract redirect models from model_mapping (target values)
  const redirectModelList = useMemo(
    () => extractRedirectModels(currentModelMapping || ""),
    [currentModelMapping],
  );

  // Extract source keys from model_mapping (models being remapped FROM)
  const redirectModelKeyList = useMemo(
    () => extractMappingSourceModels(currentModelMapping || ""),
    [currentModelMapping],
  );

  // Transform models to multi-select options
  const modelOptions = useMemo(() => {
    const allModels = new Set([...allModelsList, ...currentModelsArray]);
    return [...allModels].map((model) => ({
      value: model,
      label: model,
    }));
  }, [allModelsList, currentModelsArray]);

  const modelMappingGuardrail = useMemo<ModelMappingGuardrail>(() => {
    if (!currentModelMapping?.trim()) {
      return createEmptyModelMappingGuardrail();
    }

    try {
      const parsed = JSON.parse(currentModelMapping);
      if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
        return { ...createEmptyModelMappingGuardrail(), invalidJson: true };
      }

      const entries = Object.entries(parsed).reduce<Array<{ source: string; target: string }>>(
        (acc, [rawSource, rawTarget]) => {
          const source = String(rawSource).trim();
          const target = String(rawTarget ?? "").trim();

          if (!source || !target) {
            return acc;
          }

          acc.push({ source, target });
          return acc;
        },
        [],
      );

      const missingSourceModels = [
        ...new Set(
          entries
            .filter((entry) => Boolean(entry.source) && !currentModelsArray.includes(entry.source))
            .map((entry) => entry.source),
        ),
      ];

      const exposedTargetModels = [
        ...new Set(
          entries
            .filter((entry) => Boolean(entry.target) && currentModelsArray.includes(entry.target))
            .map((entry) => entry.target),
        ),
      ];

      return {
        invalidJson: false,
        entries,
        missingSourceModels,
        exposedTargetModels,
      };
    } catch {
      return { ...createEmptyModelMappingGuardrail(), invalidJson: true };
    }
  }, [currentModelMapping, currentModelsArray]);

  const mappingPreviewPairs =
    modelMappingGuardrail.entries.length > 0
      ? modelMappingGuardrail.entries.slice(0, 3)
      : MODEL_MAPPING_PREVIEW_FALLBACK;
  const remainingMappingCount =
    modelMappingGuardrail.entries.length > 3 ? modelMappingGuardrail.entries.length - 3 : 0;

  const upstreamUpdateMeta = useMemo(() => {
    const settings = parseSettingsRecord(currentSettings);
    const detectedModels = Array.isArray(settings.upstream_model_update_last_detected_models)
      ? settings.upstream_model_update_last_detected_models
          .map((model) => String(model || "").trim())
          .filter(Boolean)
      : [];

    return {
      lastCheckTime: settings.upstream_model_update_last_check_time,
      detectedModels: [...new Set(detectedModels)],
    };
  }, [currentSettings]);

  const upstreamDetectedModelsPreview = upstreamUpdateMeta.detectedModels.slice(
    0,
    UPSTREAM_DETECTED_MODEL_PREVIEW_LIMIT,
  );
  const upstreamDetectedModelsOmittedCount =
    upstreamUpdateMeta.detectedModels.length - upstreamDetectedModelsPreview.length;

  // Load channel data into form when editing
  useEffect(() => {
    if (!open) {
      setModelConfiguration(null);
      form.reset(CHANNEL_FORM_DEFAULT_VALUES);
      loadedForm.current = null;
      setProviderTarget(null);
      setChoosingProvider(true);
      setConfigurationSection("connection");
      setPendingErrorFocus(null);
      return;
    }
    if (isEditing && channelData?.data) {
      const isNewChannel = loadedForm.current?.channelId !== channelId;
      // Model selectors also change values without setting RHF's dirty flag.
      // Refresh untouched forms, while retaining every kind of unsaved input.
      if (!isNewChannel && loadedForm.current?.snapshot !== JSON.stringify(form.getValues())) {
        return;
      }
      const defaults = transformChannelToFormDefaults(channelData.data);
      form.reset(defaults);
      loadedForm.current = {
        channelId: channelData.data.id,
        snapshot: JSON.stringify(form.getValues()),
      };
      setProviderTarget({ kind: "builtin", type: defaults.type });
      if (isNewChannel) {
        setModelConfiguration(null);
        setChoosingProvider(false);
        setConfigurationSection("connection");
        setPendingErrorFocus(null);
      }
      // Store initial values for comparison
      initialModelsRef.current = parseModelsString(channelData.data.models || "");
      initialModelMappingRef.current = channelData.data.model_mapping || "";
      initialStatusCodeMappingRef.current = channelData.data.status_code_mapping || "";
    } else if (!isEditing) {
      form.reset(CHANNEL_FORM_DEFAULT_VALUES);
      initialModelsRef.current = [];
      initialModelMappingRef.current = "";
      initialStatusCodeMappingRef.current = "";
    }
  }, [isEditing, channelId, channelData, form, open]);

  // Validate base_url - warn if it ends with /v1
  useEffect(() => {
    if (!currentBaseUrl || !currentBaseUrl.endsWith("/v1")) return;

    // Show warning toast
    const timer = setTimeout(() => {
      toast.warning(
        t(
          "Warning: Base URL should not end with /v1. New API will handle it automatically. This may cause request failures.",
        ),
        { duration: 5000 },
      );
    }, 500);

    return () => clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentBaseUrl]);

  // Handle key deduplication
  const handleDeduplicateKeys = () => {
    const currentKey = form.getValues("key");
    if (!currentKey || currentKey.trim() === "") {
      toast.info(t("Please enter keys first"));
      return;
    }

    const result = deduplicateKeys(currentKey);

    if (result.removedCount === 0) {
      toast.info(t("No duplicate keys found"));
    } else {
      form.setValue("key", result.deduplicatedText);
      toast.success(
        t("Removed {{removed}} duplicate key(s). Before: {{before}}, After: {{after}}", {
          removed: result.removedCount,
          before: result.beforeCount,
          after: result.afterCount,
        }),
      );
    }
  };

  // Unified function to update models
  const updateModels = useCallback(
    (newModels: string[], merge: boolean = false) => {
      const finalModels = merge
        ? formatModelsArray([...currentModelsArray, ...newModels])
        : formatModelsArray(newModels);
      form.setValue("models", finalModels);
      return newModels.length;
    },
    [currentModelsArray, form],
  );

  const previewModels = !isEditing;
  const canDiscoverModels = previewModels ? canEditSensitive : canOperateChannel;
  const previewKey = isEditing ? undefined : currentKey;
  const previewRequest = useMemo<ChannelModelDiscoveryRequest>(
    () => ({
      kind: "preview",
      data: {
        type: currentType,
        key: previewKey,
        channel_id: isEditing ? channelId || undefined : undefined,
        base_url: currentBaseUrl || "",
        header_override: currentHeaderOverride,
        proxy: currentProxy,
      },
    }),
    [
      currentType,
      previewKey,
      isEditing,
      channelId,
      currentBaseUrl,
      currentHeaderOverride,
      currentProxy,
    ],
  );
  const savedRequest = useMemo<ChannelModelDiscoveryRequest>(
    () => ({ kind: "saved", channelId: channelId || 0 }),
    [channelId],
  );
  const discovery = useChannelModelDiscovery({
    enabled:
      open &&
      canDiscoverModels &&
      MODEL_FETCHABLE_TYPES.has(currentType) &&
      (!isEditing || Boolean(channelData?.data)),
    request: previewModels ? previewRequest : savedRequest,
  });
  const fetchDiscoveredModels = discovery.fetch;
  const handleFetchModels = useCallback(async () => {
    const type = form.getValues("type");
    if (!MODEL_FETCHABLE_TYPES.has(type)) {
      toast.error(t("This channel type does not support fetching models"));
      return;
    }
    if (!canDiscoverModels) {
      toast.error(t("You don't have necessary permission"));
      return;
    }
    if (!isEditing && !form.getValues("key")?.trim()) {
      form.setError("key", {
        type: "manual",
        message: ERROR_MESSAGES.REQUIRED_KEY,
      });
      setConfigurationSection("connection");
      setPendingErrorFocus("key");
      return;
    }
    await fetchDiscoveredModels();
  }, [isEditing, canDiscoverModels, form, t, fetchDiscoveredModels]);

  // Handle model operations
  const handleFillRelatedModels = useCallback(() => {
    if (!basicModels.length) {
      toast.info(t("No related models available for this channel type"));
      return;
    }
    updateModels(basicModels);
    toast.success(t("Filled {{count}} related model(s)", { count: basicModels.length }));
  }, [basicModels, updateModels, t]);

  const handleClearModels = useCallback(() => {
    form.setValue("models", "");
    toast.success(t("Cleared all models"));
  }, [form, t]);

  const handleCopyModels = useCallback(async () => {
    const models = form.getValues("models");
    if (!models?.trim()) {
      toast.info(t("No models to copy"));
      return;
    }
    await copyToClipboard(models);
  }, [form, copyToClipboard, t]);

  // Handle adding prefill group models
  const handleAddPrefillGroup = useCallback(
    (group: { id: number; name: string; items: string | string[] }) => {
      try {
        const items = Array.isArray(group.items) ? group.items : JSON.parse(group.items);

        if (!Array.isArray(items)) {
          throw new Error("Invalid items format");
        }

        const count = updateModels(items, true);
        toast.success(
          t('Added {{count}} models from "{{name}}"', {
            count,
            name: group.name,
          }),
        );
      } catch {
        toast.error(t("Failed to parse group items"));
      }
    },
    [updateModels, t],
  );

  // Handle model selection change from MultiSelect
  const handleModelsChange = useCallback(
    (selected: string[]) => {
      form.setValue("models", selected.join(","));
    },
    [form],
  );

  // Handle successful submission
  const handleSuccess = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() });
    if (channelId) {
      queryClient.invalidateQueries({
        queryKey: channelsQueryKeys.detail(channelId),
      });
    }
    onOpenChange(false);
    setOpen(null);
  }, [channelId, queryClient, onOpenChange, setOpen]);

  // Show missing models confirmation dialog
  const confirmMissingModelMappings = useCallback(
    (missingModels: string[]): Promise<MissingModelsAction> => {
      return new Promise((resolve) => {
        setMissingModelsList(missingModels);
        setMissingModelsDialogOpen(true);
        missingModelsResolveRef.current = resolve;
      });
    },
    [],
  );

  // Handle missing models dialog action
  const handleMissingModelsAction = useCallback((action: MissingModelsAction) => {
    setMissingModelsDialogOpen(false);
    if (missingModelsResolveRef.current) {
      missingModelsResolveRef.current(action);
      missingModelsResolveRef.current = null;
    }
  }, []);

  const confirmStatusCodeRisk = useCallback(
    (detailItems: string[]): Promise<boolean> =>
      new Promise((resolve) => {
        statusCodeRiskResolveRef.current = resolve;
        setStatusCodeRiskDetailItems(detailItems);
        setStatusCodeRiskOpen(true);
      }),
    [],
  );

  const handleStatusCodeRiskAction = useCallback((confirmed: boolean) => {
    setStatusCodeRiskOpen(false);
    setStatusCodeRiskDetailItems([]);
    if (statusCodeRiskResolveRef.current) {
      statusCodeRiskResolveRef.current(confirmed);
      statusCodeRiskResolveRef.current = null;
    }
  }, []);

  useEffect(() => {
    return () => {
      if (statusCodeRiskResolveRef.current) {
        statusCodeRiskResolveRef.current(false);
        statusCodeRiskResolveRef.current = null;
      }
    };
  }, []);

  const channelMutation = useChannelMutateForm({
    currentRow,
    isEditing,
    isMultiKeyChannel,
    onSuccess: handleSuccess,
  });

  const isSubmitting = channelMutation.isPending || form.formState.isSubmitting;

  // Submit handler
  const onSubmit = useCallback(
    async (data: ChannelFormValues) => {
      if (isEditing && !channelData?.data) return;
      if (!isEditing && (!providerTarget || !canEditSensitive)) return;
      // Validate key is required when creating
      if (!isEditing && !data.key?.trim()) {
        form.setError("key", {
          type: "manual",
          message: ERROR_MESSAGES.REQUIRED_KEY,
        });
        setConfigurationSection("connection");
        setPendingErrorFocus("key");
        return;
      }

      if (sensitiveLocked) {
        const dirtyFields = form.formState.dirtyFields as Partial<
          Record<keyof ChannelFormValues, unknown>
        >;
        const hasSensitiveChanges = SENSITIVE_FORM_FIELDS.some((field) =>
          Boolean(dirtyFields[field]),
        );
        if (hasSensitiveChanges) {
          toast.error(t("You do not have permission to edit sensitive channel settings."));
          return;
        }
      }

      // Validate status_code_mapping entries
      if (data.status_code_mapping?.trim()) {
        const invalidEntries = collectInvalidStatusCodeEntries(data.status_code_mapping);
        if (invalidEntries.length > 0) {
          const message = t("Invalid status code mapping entries: {{entries}}", {
            entries: invalidEntries.join(", "),
          });
          form.setError("status_code_mapping", { type: "manual", message });
          setConfigurationSection("request");
          setPendingErrorFocus("status_code_mapping");
          toast.error(message);
          return;
        }

        const riskyRedirects = collectNewDisallowedStatusCodeRedirects(
          initialStatusCodeMappingRef.current,
          data.status_code_mapping,
        );
        if (riskyRedirects.length > 0) {
          const confirmed = await confirmStatusCodeRisk(riskyRedirects);
          if (!confirmed) return;
        }
      }

      // Validate model_mapping JSON format
      const hasModelMapping =
        typeof data.model_mapping === "string" && data.model_mapping.trim() !== "";
      const modelMappingValue = data.model_mapping || "";

      if (hasModelMapping) {
        const validation = validateModelMappingJson(modelMappingValue);
        if (!validation.valid) {
          form.setError("model_mapping", {
            type: "manual",
            message: t(validation.error || "Invalid model mapping"),
          });
          setConfigurationSection("routing");
          setPendingErrorFocus("model_mapping");
          handleServerError(validation, t(validation.error || "Invalid model mapping"));
          return;
        }
      }

      // Normalize models array
      const normalizedModels = parseModelsString(data.models || "");

      // Check for missing models in model_mapping
      if (hasModelMapping) {
        const missingModels = findMissingModelsInMapping(modelMappingValue, normalizedModels);

        const shouldPromptMissing =
          missingModels.length > 0 &&
          hasModelConfigChanged(
            normalizedModels,
            data.model_mapping || "",
            initialModelsRef.current,
            initialModelMappingRef.current,
          );

        if (shouldPromptMissing) {
          const confirmAction = await confirmMissingModelMappings(missingModels);
          if (confirmAction === "cancel") {
            return;
          }
          if (confirmAction === "add") {
            const updatedModels = [...new Set([...normalizedModels, ...missingModels])];
            data.models = formatModelsArray(updatedModels);
            form.setValue("models", data.models);
          }
        }
      }

      try {
        await channelMutation.mutateAsync(data);
      } catch {
        // The mutation reports the server error; keep the draft open for correction.
      }
    },
    [
      isEditing,
      providerTarget,
      canEditSensitive,
      sensitiveLocked,
      channelData,
      form,
      confirmMissingModelMappings,
      confirmStatusCodeRisk,
      channelMutation,
      t,
    ],
  );

  const onInvalid: SubmitErrorHandler<ChannelFormValues> = useCallback(
    (errors) => {
      const field = Object.keys(errors)[0];
      if (field) {
        setConfigurationSection(getChannelConfigurationSection(field));
        setPendingErrorFocus(field);
      }
      toast.error(t("Please fix the highlighted fields before saving"));
    },
    [t],
  );

  useEffect(() => {
    if (!pendingErrorFocus || showProviderPicker) return;
    const frame = window.requestAnimationFrame(() => {
      const panel = channelFormRef.current?.querySelector('[role="tabpanel"]:not([hidden])');
      const invalid = panel?.querySelector<HTMLElement>('[aria-invalid="true"]');
      const item = invalid?.closest<HTMLElement>('[data-slot="form-item"]');
      const controls =
        'input:not([type="hidden"]), textarea, button, [contenteditable="true"], [tabindex]:not([tabindex="-1"])';
      const focusTarget = invalid?.matches(controls)
        ? invalid
        : item?.querySelector<HTMLElement>(controls);
      item?.scrollIntoView({ block: "center", behavior: "smooth" });
      focusTarget?.focus({ preventScroll: true });
      setPendingErrorFocus(null);
    });
    return () => window.cancelAnimationFrame(frame);
  }, [pendingErrorFocus, configurationSection, showProviderPicker]);

  // Handle drawer close
  const handleOpenChange = useCallback<NonNullable<ComponentProps<typeof Sheet>["onOpenChange"]>>(
    (v, details) => {
      if (!v && isSubmitting) return;
      if (!v && showProviderPicker && providerTarget && details.reason === "escape-key") {
        details.cancel();
        setChoosingProvider(false);
        return;
      }
      onOpenChange(v);
      if (!v) {
        form.reset(CHANNEL_FORM_DEFAULT_VALUES);
        setClipboardConnectionInfo(null);
      }
    },
    [onOpenChange, form, isSubmitting, showProviderPicker, providerTarget],
  );

  const protocolFieldProps = {
    form,
    t,
    sensitiveLocked,
    currentType,
    currentHttpProtocol,
  };

  const notesFields = (
    <div
      role="group"
      aria-label={t("Internal Notes")}
      className={channelConfigurationBlockClassName(
        configuration.blocks.internalNotes,
        "flex flex-col gap-4",
      )}
    >
      <SubHeading
        title={t("Internal Notes")}
        status={configuration.blocks.internalNotes}
        icon={<FileText className="h-3.5 w-3.5" />}
        iconTone="chart-3"
      />
      <div className="grid gap-4 sm:grid-cols-2">
        <FormField
          control={form.control}
          name="tag"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t("Tag")}</FormLabel>
              <FormControl>
                <Input placeholder={t(FIELD_PLACEHOLDERS.TAG)} {...field} />
              </FormControl>
              <FormDescription>{t(FIELD_DESCRIPTIONS.TAG)}</FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name="remark"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t("Remark")}</FormLabel>
              <FormControl>
                <Textarea placeholder={t(FIELD_PLACEHOLDERS.REMARK)} rows={2} {...field} />
              </FormControl>
              <FormDescription>{t(FIELD_DESCRIPTIONS.REMARK)}</FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>
    </div>
  );

  const routingFields = (
    <div
      role="group"
      aria-label={t("Routing Strategy")}
      className={channelConfigurationBlockClassName(
        configuration.blocks.routingStrategy,
        "flex flex-col gap-4",
      )}
    >
      <SubHeading
        title={t("Routing Strategy")}
        status={configuration.blocks.routingStrategy}
        icon={<Route className="h-3.5 w-3.5" />}
        iconTone="info"
      />
      <div className="grid gap-4 sm:grid-cols-2">
        <FormField
          control={form.control}
          name="priority"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t("Priority")}</FormLabel>
              <FormControl>
                <Input
                  type="number"
                  placeholder="0"
                  {...field}
                  onChange={(e) => field.onChange(Number(e.target.value))}
                />
              </FormControl>
              <FormDescription>{t(FIELD_DESCRIPTIONS.PRIORITY)}</FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name="weight"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t("Weight")}</FormLabel>
              <FormControl>
                <Input
                  type="number"
                  placeholder="0"
                  {...field}
                  onChange={(e) => field.onChange(Number(e.target.value))}
                />
              </FormControl>
              <FormDescription>{t(FIELD_DESCRIPTIONS.WEIGHT)}</FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>

      <FormField
        control={form.control}
        name="test_model"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("Test Model")}</FormLabel>
            <FormControl>
              <Input placeholder={t(FIELD_PLACEHOLDERS.TEST_MODEL)} {...field} />
            </FormControl>
            <FormDescription>{t(FIELD_DESCRIPTIONS.TEST_MODEL)}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={form.control}
        name="auto_ban"
        render={({ field }) => (
          <FormItem className="flex items-center justify-between">
            <div className="space-y-0.5">
              <FormLabel>{t("Auto Ban")}</FormLabel>
              <FormDescription>{t(FIELD_DESCRIPTIONS.AUTO_BAN)}</FormDescription>
            </div>
            <FormControl>
              <Switch
                checked={field.value === 1}
                onCheckedChange={(checked) => field.onChange(checked ? 1 : 0)}
              />
            </FormControl>
          </FormItem>
        )}
      />
    </div>
  );

  const modelSections = buildChannelModelSections({
    form,
    currentType,
    t,
    sensitiveLocked,
    configuration,
    channelConfigurationBlockClassName,
    CardHeading,
    upstreamModelUpdateCheckEnabled,
    formatUnixTime,
    upstreamUpdateMeta,
    upstreamDetectedModelsPreview,
    upstreamDetectedModelsOmittedCount,
    modelMappingGuardrail,
    mappingPreviewPairs,
    remainingMappingCount,
    isSubmitting,
    currentModelsArray,
    modelOptions,
    formatModelNames,
    updateModels,
    isEditing,
    initialModelsRef,
    discovery,
    handleFetchModels,
    redirectModelList,
    redirectModelKeyList,
    previewModels,
    isBatchMode,
    basicModels,
    canDiscoverModels,
    handleFillRelatedModels,
    handleCopyModels,
    handleClearModels,
    prefillGroups,
    handleAddPrefillGroup,
    isLoadingGroups,
    groupOptions,
    handleModelsChange,
    setModelConfiguration,
  });
  const upstreamModelDetectionFields = modelSections.upstreamModelDetectionFields;
  const modelMappingFields = modelSections.modelMappingFields;
  const modelsSection = modelSections.modelsSection;
  const connectionSections = buildChannelConnectionSections({
    form,
    currentType,
    t,
    sensitiveLocked,
    isEditing,
    configuration,
    channelConfigurationBlockClassName,
    CardHeading,
    FIELD_PLACEHOLDERS,
    FIELD_DESCRIPTIONS,
    isSubmitting,
    setParamOverrideEditorOpen,
    currentKey,
    isBatchMode,
    keyMode,
    isMultiKeyChannel,
    canRevealChannelKey,
    handleDeduplicateKeys,
    handleRevealKey,
    isChannelKeyLoading,
    verification,
    channelKey,
    copyToClipboard,
    multiKeyMode,
    multiKeyType,
    addModeOptions,
    SubHeading,
    getKeyPromptForType,
  });
  const basicSection = connectionSections.basicSection;
  const overrideFields = connectionSections.overrideFields;
  const fieldPassthroughFields = connectionSections.fieldPassthroughFields;
  const connectionSection = connectionSections.connectionSection;

  let formContent: ReactNode;
  if (!isEditing && !providerTarget) {
    formContent = null;
  } else if (isEditing && isChannelError && !channelData?.data) {
    formContent = (
      <ErrorState
        title={t("Failed to load channel")}
        description={getServerErrorMessage(channelError, t("Failed to load channel"))}
        onRetry={() => {
          void refetchChannel();
        }}
      />
    );
  } else if (isChannelDetailLoading) {
    formContent = <ChannelEditorLoadingState />;
  } else {
    formContent = (
      <ChannelConfiguration
        section={configurationSection}
        onSectionChange={setConfigurationSection}
        statuses={configuration.sections}
        connection={
          <>
            {basicSection}
            {connectionSection}
          </>
        }
        models={modelsSection}
        routing={
          <>
            {modelMappingFields}
            {routingFields}
          </>
        }
        request={
          <>
            {overrideFields}
            <div
              role="group"
              aria-label={t("Request processing")}
              className={channelConfigurationBlockClassName(
                configuration.blocks.requestProcessing,
                "space-y-4",
              )}
            >
              <CardHeading
                status={configuration.blocks.requestProcessing}
                title={t("Request processing")}
                icon={<Settings className="size-4" />}
              />
              <fieldset disabled={sensitiveLocked} className="space-y-4 disabled:opacity-60">
                <ChannelFormatField {...protocolFieldProps} />
                <ChannelThinkingField {...protocolFieldProps} />
                <ChannelPassThroughField {...protocolFieldProps} />
                <ChannelSystemPromptField {...protocolFieldProps} />
                <ChannelSystemPromptOverrideField {...protocolFieldProps} />
              </fieldset>
            </div>
            {fieldPassthroughFields}
          </>
        }
        other={
          <>
            <div
              role="group"
              aria-label={t("Channel Extra Settings")}
              className={channelConfigurationBlockClassName(
                configuration.blocks.extraSettings,
                "space-y-4",
              )}
            >
              <CardHeading
                status={configuration.blocks.extraSettings}
                title={t("Channel Extra Settings")}
                icon={<Settings className="size-4" />}
              />
              <fieldset disabled={sensitiveLocked} className="space-y-4 disabled:opacity-60">
                <ChannelProxyField {...protocolFieldProps} />
                <ChannelHttpProtocolField {...protocolFieldProps} />
                <ChannelHttpShardsField {...protocolFieldProps} />
              </fieldset>
            </div>
            {upstreamModelDetectionFields}
            {notesFields}
          </>
        }
      />
    );
  }
  let description = t("Configure the connection and models, then create the channel.");
  if (showProviderPicker && providerTarget) {
    description = `${t("Current:")} ${providerLabel}`;
  } else if (isEditing) {
    description = t("Update channel configuration and click save when you're done.");
  } else if (showProviderPicker) {
    description = t("Choose a provider or plugin to configure your channel.");
  }

  return (
    <>
      <Sheet open={open} onOpenChange={handleOpenChange}>
        <SheetContent side={drawerSide} className={sideDrawerContentClassName("sm:max-w-7xl")}>
          <SheetHeader className={sideDrawerHeaderClassName("pr-12 sm:pr-14")}>
            <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div className="min-w-0 flex-1">
                <div className="flex min-w-0 items-center gap-2 sm:gap-3">
                  <SheetTitle className="flex shrink-0 items-center gap-2 sm:gap-3">
                    <IconBadge tone="info" size="title">
                      <Server className="size-5" />
                    </IconBadge>
                    <span>{isEditing ? t("Edit Channel") : t("Create Channel")}</span>
                  </SheetTitle>
                  {(!showProviderPicker || providerTarget) && (
                    <Button
                      ref={providerControlRef}
                      type="button"
                      variant="outline"
                      aria-label={
                        showProviderPicker ? t("Back to configuration") : t("Change provider")
                      }
                      aria-description={providerLabel}
                      title={providerLabel}
                      className="min-w-0 shrink gap-2 sm:max-w-md"
                      disabled={
                        isSubmitting ||
                        (!showProviderPicker &&
                          (!canEditSensitive || (isEditing && !channelData?.data)))
                      }
                      onClick={() => setChoosingProvider(!showProviderPicker)}
                    >
                      {showProviderPicker ? (
                        <>
                          <ArrowLeft className="size-4" aria-hidden="true" />
                          <span className="shrink-0 sm:hidden">{t("Back")}</span>
                          <span className="hidden shrink-0 sm:inline">
                            {t("Back to configuration")}
                          </span>
                        </>
                      ) : (
                        <ChannelTypeLogo type={currentType} size={18} />
                      )}
                      <span className="min-w-0 truncate">{providerLabel}</span>
                      {!showProviderPicker && (
                        <>
                          <span className="hidden shrink-0 sm:inline">{t("Change provider")}</span>
                          <ChevronDown className="size-4" aria-hidden="true" />
                        </>
                      )}
                    </Button>
                  )}
                </div>
                {isEditing && channelData?.data && (
                  <Badge variant="secondary" className="mt-2">
                    {t(
                      CHANNEL_STATUS_LABELS[currentStatus as keyof typeof CHANNEL_STATUS_LABELS] ||
                        "Unknown",
                    )}
                  </Badge>
                )}
                <SheetDescription
                  className={cn("mt-1", showProviderPicker && providerTarget && "truncate")}
                  title={showProviderPicker && providerTarget ? description : undefined}
                >
                  {description}
                </SheetDescription>
              </div>
              {!isEditing && !showProviderPicker && (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="shrink-0"
                  onClick={pasteConnectionInfoFromClipboard}
                >
                  <ClipboardPaste className="size-4" />
                  <span>{t("Paste Connection Info")}</span>
                </Button>
              )}
            </div>
          </SheetHeader>

          {showProviderPicker && (
            <ChannelProviderPicker
              disabled={isSubmitting || !canEditSensitive}
              onSelect={selectProvider}
            />
          )}

          {sensitiveLocked && (
            <Alert className="border-amber-200 bg-amber-50 text-amber-900 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-50">
              <AlertDescription>
                {t("Sensitive channel settings are read-only for your account.")}{" "}
                {t(
                  "You can still edit non-sensitive operations fields such as models, groups, priority, and weight.",
                )}
              </AlertDescription>
            </Alert>
          )}

          {!isEditing && !showProviderPicker && clipboardConnectionInfo && (
            <Alert>
              <AlertDescription className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <span>{t("Connection info detected in clipboard")}</span>
                <span className="flex shrink-0 gap-2">
                  <Button
                    type="button"
                    size="sm"
                    onClick={() => applyConnectionInfo(clipboardConnectionInfo)}
                  >
                    {t("Fill in")}
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => setClipboardConnectionInfo(null)}
                  >
                    {t("Ignore")}
                  </Button>
                </span>
              </AlertDescription>
            </Alert>
          )}

          <Form {...form}>
            <form
              id="channel-form"
              ref={channelFormRef}
              onSubmit={form.handleSubmit(onSubmit, onInvalid)}
              className={sideDrawerFormClassName(
                cn(
                  "gap-5",
                  (!isEditing || (!isChannelDetailLoading && channelData?.data)) &&
                    "overflow-hidden",
                  showProviderPicker && "hidden",
                ),
              )}
              hidden={showProviderPicker}
            >
              {formContent}
            </form>
          </Form>

          <SheetFooter className={sideDrawerFooterClassName()}>
            {showProviderPicker && providerTarget ? (
              <Button
                type="button"
                variant="outline"
                disabled={isSubmitting}
                onClick={() => setChoosingProvider(false)}
              >
                {t("Cancel")}
              </Button>
            ) : (
              <SheetClose render={<Button variant="outline" disabled={isSubmitting} />}>
                {t("Cancel")}
              </SheetClose>
            )}
            {!showProviderPicker && (
              <Button
                form="channel-form"
                type="submit"
                disabled={
                  isSubmitting ||
                  (!isEditing && !canEditSensitive) ||
                  (isEditing && !channelData?.data)
                }
              >
                {isSubmitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {isEditing ? t("Update Channel") : t("Create Channel")}
              </Button>
            )}
          </SheetFooter>
        </SheetContent>
      </Sheet>

      {open && modelConfiguration && (
        <ConfigureModelsDialog
          open
          models={currentModelsArray}
          onOpenChange={(nextOpen) => {
            if (!nextOpen) setModelConfiguration(null);
          }}
          onApply={handleModelsChange}
        />
      )}

      {paramOverrideEditorOpen && !sensitiveLocked && (
        <ParamOverrideEditorDialog
          open={paramOverrideEditorOpen}
          value={formValues.param_override || ""}
          onOpenChange={setParamOverrideEditorOpen}
          onSave={(nextValue) => {
            form.setValue("param_override", nextValue, {
              shouldDirty: true,
              shouldValidate: true,
            });
          }}
        />
      )}

      <SecureVerificationDialog {...verification.dialogProps} />

      {/* Missing Models Confirmation Dialog */}
      <MissingModelsConfirmationDialog
        open={missingModelsDialogOpen}
        missingModels={missingModelsList}
        onConfirm={handleMissingModelsAction}
        onOpenChange={setMissingModelsDialogOpen}
      />

      <StatusCodeRiskDialog
        open={statusCodeRiskOpen}
        onOpenChange={(v) => {
          if (!v) handleStatusCodeRiskAction(false);
        }}
        detailItems={statusCodeRiskDetailItems}
        onConfirm={() => handleStatusCodeRiskAction(true)}
      />
    </>
  );
}
