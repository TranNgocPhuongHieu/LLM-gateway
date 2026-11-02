import type { TFunction } from "i18next";
import type { ReactNode } from "react";
import type { UseFormReturn } from "react-hook-form";
import { ArrowRight, Copy, Eraser, FileText, RefreshCw, Settings, Sparkles } from "lucide-react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { FormControl, FormDescription, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { PopoverDescription, PopoverTitle } from "@/components/ui/popover";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { getServerErrorMessage } from "@/lib/server-error-message";
import { ChannelModelsSection } from "./sections";
import { ChannelConfigurationStatusIndicator } from "./channel-configuration";
import { EmptyState } from "@/components/empty-state";
import { ErrorState } from "@/components/error-state";
import { LearnMore } from "@/components/learn-more";
import { LoadingState } from "@/components/loading-state";
import { MultiSelect } from "@/components/multi-select";
import { UpstreamModelSelection } from "../upstream-model-selection";
import type { ChannelFormValues } from "../../lib";
import { FIELD_DESCRIPTIONS, FIELD_PLACEHOLDERS, MODEL_FETCHABLE_TYPES } from "../../constants";
import { ModelMappingEditor } from "../model-mapping-editor";

export type ChannelModelSectionsProps = {
  form: UseFormReturn<ChannelFormValues>;
  currentType: number;
  t: TFunction;
  [key: string]: any;
};

export function buildChannelModelSections(props: ChannelModelSectionsProps): Record<string, ReactNode> {
  const { form, currentType, t } = props;
  const { sensitiveLocked, configuration, channelConfigurationBlockClassName, CardHeading,
    upstreamModelUpdateCheckEnabled, formatUnixTime, upstreamUpdateMeta, upstreamDetectedModelsPreview,
    upstreamDetectedModelsOmittedCount, modelMappingGuardrail, mappingPreviewPairs, remainingMappingCount,
    isSubmitting, currentModelsArray, modelOptions, formatModelNames, updateModels, isEditing,
    initialModelsRef, discovery, handleFetchModels, redirectModelList, redirectModelKeyList,
    previewModels, isBatchMode, basicModels, canDiscoverModels, handleFillRelatedModels,
    handleCopyModels, handleClearModels, prefillGroups, handleAddPrefillGroup, isLoadingGroups,
    groupOptions, handleModelsChange, setModelConfiguration } = props;

  const upstreamModelDetectionFields = MODEL_FETCHABLE_TYPES.has(currentType) && (
    <div
      role="group"
      aria-label={t("Upstream Model Detection Settings")}
      className={channelConfigurationBlockClassName(
        configuration.blocks.upstreamModelDetection,
        "flex flex-col gap-4",
      )}
    >
      <CardHeading
        title={t("Upstream Model Detection Settings")}
        status={configuration.blocks.upstreamModelDetection}
        icon={<RefreshCw className="h-4 w-4" />}
        iconTone="info"
      />
      <fieldset disabled={sensitiveLocked} className="space-y-4 disabled:opacity-60">
        <div className="divide-border space-y-0 divide-y border-y">
          <FormField
            control={form.control}
            name="upstream_model_update_check_enabled"
            render={({ field }) => (
              <FormItem className="flex items-center justify-between px-4 py-3">
                <div className="space-y-0.5">
                  <FormLabel>{t("Upstream Model Update Check")}</FormLabel>
                  <FormDescription>
                    {t("Periodically check for upstream model changes")}
                  </FormDescription>
                  <FormMessage />
                </div>
                <FormControl>
                  <Switch
                    disabled={sensitiveLocked}
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="upstream_model_update_auto_sync_enabled"
            render={({ field }) => (
              <FormItem className="flex items-center justify-between px-4 py-3">
                <div className="space-y-0.5">
                  <FormLabel>{t("Auto Sync Upstream Models")}</FormLabel>
                  <FormDescription>
                    {t("Automatically sync model list when upstream changes are detected")}
                  </FormDescription>
                </div>
                <FormControl>
                  <Switch
                    checked={field.value}
                    disabled={sensitiveLocked || !upstreamModelUpdateCheckEnabled}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </FormItem>
            )}
          />
        </div>
        <FormField
          control={form.control}
          name="upstream_model_update_ignored_models"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t("Ignored upstream models")}</FormLabel>
              <FormControl>
                <Input
                  placeholder={t("e.g., gpt-4.1-nano,regex:^claude-.*$,regex:^sora-.*$")}
                  {...field}
                />
              </FormControl>
              <FormDescription>
                {t(
                  "Comma-separated exact model names. Prefix with regex: to ignore by regular expression.",
                )}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <div className="text-muted-foreground space-y-2 border-t pt-3 text-xs">
          <div>
            <span className="text-foreground font-medium">{t("Last check time")}:</span>{" "}
            {formatUnixTime(upstreamUpdateMeta.lastCheckTime)}
          </div>
          <div>
            <span className="text-foreground font-medium">
              {t("Last detected addable models")}:
            </span>{" "}
            {upstreamUpdateMeta.detectedModels.length === 0 ? (
              t("None")
            ) : (
              <>
                <span className="break-all">{upstreamDetectedModelsPreview.join(", ")}</span>
                {upstreamDetectedModelsOmittedCount > 0 && (
                  <span className="ml-1">
                    {t("({{total}} total, {{omit}} omitted)", {
                      total: upstreamUpdateMeta.detectedModels.length,
                      omit: upstreamDetectedModelsOmittedCount,
                    })}
                  </span>
                )}
              </>
            )}
          </div>
        </div>
      </fieldset>
    </div>
  );

  const modelMappingFields = (
    <div
      role="group"
      aria-label={t("Model Mapping")}
      className={channelConfigurationBlockClassName(configuration.blocks.modelMapping)}
    >
      <FormField
        control={form.control}
        name="model_mapping"
        render={({ field }) => (
          <FormItem className="space-y-3">
            <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
              <div className="space-y-1">
                <div className="flex items-center gap-2">
                  <FormLabel className="mb-0">{t("Model Mapping")}</FormLabel>
                  <ChannelConfigurationStatusIndicator status={configuration.blocks.modelMapping} />
                  <LearnMore
                    triggerProps={{
                      type: "button",
                      "aria-label": t("How model mapping works"),
                      className: "size-4 rounded-full",
                    }}
                    contentProps={{
                      collisionPadding: 16,
                      className: "w-96 max-w-[calc(100vw-2rem)] gap-3 p-4 text-sm leading-6",
                    }}
                  >
                    <PopoverTitle className="text-foreground">{t("Request flow")}</PopoverTitle>
                    <div className="space-y-2 font-mono">
                      {mappingPreviewPairs.map((pair: { source: string; target: string }) => (
                        <div
                          key={`${pair.source}-${pair.target}`}
                          className="grid grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-start gap-3"
                        >
                          <span className="min-w-0 wrap-anywhere">{pair.source}</span>
                          <ArrowRight className="mt-1 size-4 shrink-0" aria-hidden="true" />
                          <span className="min-w-0 wrap-anywhere">{pair.target}</span>
                        </div>
                      ))}
                      {remainingMappingCount > 0 && (
                        <div>
                          +{remainingMappingCount} {t("more mapping")}
                          {remainingMappingCount > 1 ? "s" : ""}
                        </div>
                      )}
                    </div>
                    <PopoverDescription className="leading-6">
                      {t(
                        "Users call the model on the left. The platform forwards the request to the upstream model on the right.",
                      )}
                    </PopoverDescription>
                  </LearnMore>
                </div>
                <FormDescription>{t(FIELD_DESCRIPTIONS.MODEL_MAPPING)}</FormDescription>
              </div>
            </div>
            <FormControl>
              <ModelMappingEditor
                value={field.value || ""}
                onChange={field.onChange}
                disabled={isSubmitting}
                sourceModelOptions={currentModelsArray}
                targetModelOptions={modelOptions.map((option: { value: string }) => option.value)}
              />
            </FormControl>
            {modelMappingGuardrail.invalidJson && (
              <Alert variant="destructive">
                <AlertDescription>
                  {t("Model Mapping must be a JSON object like")}{" "}
                  <code className="font-mono">{'{"gpt-4":"Azure-GPT4"}'}</code>
                  {t(". Please fix the JSON before saving.")}
                </AlertDescription>
              </Alert>
            )}
            {modelMappingGuardrail.missingSourceModels.length > 0 && (
              <Alert className="border-amber-200 bg-amber-50 text-amber-900 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-50">
                <AlertDescription className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <span>
                    {t("Add")} {formatModelNames(modelMappingGuardrail.missingSourceModels)}{" "}
                    {t(
                      "to the Models list so users can use them before the mapping sends traffic upstream.",
                    )}
                  </span>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      updateModels([
                        ...currentModelsArray,
                        ...modelMappingGuardrail.missingSourceModels,
                      ]);
                    }}
                  >
                    {t("Add missing models")}
                  </Button>
                </AlertDescription>
              </Alert>
            )}
            <FormMessage />
          </FormItem>
        )}
      />
    </div>
  );

  const modelsSection = (
    <div className="scroll-mt-4">
      <ChannelModelsSection>
        <div className="space-y-5">
          <div className="border-border/60 bg-muted/10 rounded-lg border p-4">
            <FormField
              control={form.control}
              name="models"
              render={() => (
                <FormItem role="group" aria-label={t("Models")} className="space-y-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0 space-y-1">
                      <FormLabel required>{t("Models")}</FormLabel>
                      <FormDescription>{t(FIELD_DESCRIPTIONS.MODELS)}</FormDescription>
                    </div>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => setModelConfiguration({})}
                      disabled={currentModelsArray.length === 0}
                    >
                      <Settings className="mr-2 h-4 w-4" aria-hidden="true" />
                      {t("Configure Models")}
                    </Button>
                  </div>
                  <FormControl>
                    <MultiSelect
                      options={modelOptions}
                      selected={currentModelsArray}
                      onChange={handleModelsChange}
                      placeholder={t("Select models or add custom ones")}
                      allowCreate
                      createLabel='Add custom model "{{value}}"'
                      maxVisibleChips={8}
                      copyChipOnClick
                    />
                  </FormControl>
                  {modelMappingGuardrail.exposedTargetModels.length > 0 && (
                    <Alert className="border-amber-200 bg-amber-50 text-amber-900 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-50">
                      <AlertDescription className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                        <span>
                          {t("The mapped upstream model(s)")}{" "}
                          {formatModelNames(modelMappingGuardrail.exposedTargetModels)}{" "}
                          {t(
                            "are also listed here. Remove them from Models to keep the `/v1/models` response user-friendly and hide vendor-specific names.",
                          )}
                        </span>
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          onClick={() => {
                            const hiddenTargets = new Set(
                              modelMappingGuardrail.exposedTargetModels,
                            );
                            updateModels(
                              currentModelsArray.filter((model: string) => !hiddenTargets.has(model)),
                            );
                          }}
                        >
                          {t("Remove mapped targets")}
                        </Button>
                      </AlertDescription>
                    </Alert>
                  )}
                  <FormMessage />
                </FormItem>
              )}
            />

            {MODEL_FETCHABLE_TYPES.has(currentType) && (
              <div aria-live="polite" className="mt-4 space-y-3">
                {discovery.status === "loading" && (
                  <LoadingState className="min-h-0 py-4" message={t("Fetching models...")} />
                )}
                {discovery.status === "error" && (
                  <ErrorState
                    className="min-h-0 p-3"
                    title={t("Failed to fetch models")}
                    description={getServerErrorMessage(
                      discovery.error,
                      t("Failed to fetch models"),
                    )}
                    onRetry={() => {
                      void handleFetchModels();
                    }}
                  />
                )}
                {discovery.status === "stale" && (
                  <Alert>
                    <AlertDescription>
                      {t("Connection settings changed. Fetch models again to refresh the list.")}
                      <Button type="button" variant="outline" size="sm" onClick={handleFetchModels}>
                        {t("Fetch Models")}
                      </Button>
                    </AlertDescription>
                  </Alert>
                )}
                {discovery.status === "success" && discovery.models.length === 0 && (
                  <EmptyState
                    className="min-h-0 p-3"
                    title={t("No models returned by the upstream")}
                    description={t("You can add models manually or try fetching again.")}
                    action={
                      <Button type="button" variant="outline" size="sm" onClick={handleFetchModels}>
                        {t("Retry")}
                      </Button>
                    }
                  />
                )}
                {discovery.status === "success" && discovery.models.length > 0 && (
                  <UpstreamModelSelection
                    models={discovery.models}
                    selected={currentModelsArray}
                    existingModels={isEditing ? initialModelsRef.current : currentModelsArray}
                    onChange={handleModelsChange}
                    showChanges={isEditing}
                    redirectModels={redirectModelList}
                    redirectSourceModels={redirectModelKeyList}
                  />
                )}
                {isEditing && !previewModels && (
                  <p className="text-muted-foreground text-xs">
                    {t("Model discovery uses the saved channel connection settings.")}
                  </p>
                )}
                {!isEditing && isBatchMode && (
                  <p className="text-muted-foreground text-xs">
                    {t("Model discovery uses the first key; other keys are not tested.")}
                  </p>
                )}
              </div>
            )}

            <Separator className="my-4" />

            <div className="space-y-3">
              <div>
                <p className="text-sm font-medium">{t("Quick actions")}</p>
                <p className="text-muted-foreground text-xs">
                  {t("Use presets or upstream discovery to populate the model list faster.")}
                </p>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={handleFillRelatedModels}
                  disabled={!basicModels.length}
                >
                  <FileText className="mr-2 h-4 w-4" aria-hidden="true" />
                  {t("Fill Related Models")}
                </Button>
                {MODEL_FETCHABLE_TYPES.has(currentType) && (
                  <>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={handleFetchModels}
                      disabled={!canDiscoverModels || discovery.status === "loading"}
                    >
                      <Sparkles className="mr-2 h-4 w-4" aria-hidden="true" />
                      {t("Fetch from Upstream")}
                    </Button>
                    {!canDiscoverModels && (
                      <span className="text-muted-foreground basis-full text-xs">
                        {t("No permission to perform this action")}
                      </span>
                    )}
                  </>
                )}
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={handleCopyModels}
                  disabled={currentModelsArray.length === 0}
                >
                  <Copy className="mr-2 h-4 w-4" aria-hidden="true" />
                  {t("Copy All")}
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={handleClearModels}
                  disabled={currentModelsArray.length === 0}
                >
                  <Eraser className="mr-2 h-4 w-4" aria-hidden="true" />
                  {t("Clear All")}
                </Button>
              </div>
              {prefillGroups.length > 0 && (
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-muted-foreground text-xs">{t("Preset groups")}:</span>
                  {prefillGroups.map((group: { id: string; name: string }) => (
                    <Button
                      key={group.id}
                      type="button"
                      variant="secondary"
                      size="sm"
                      onClick={() => handleAddPrefillGroup(group)}
                    >
                      {group.name}
                    </Button>
                  ))}
                </div>
              )}
            </div>
          </div>

          <div className="border-border/60 rounded-lg border p-4">
            <FormField
              control={form.control}
              name="group"
              render={({ field }) => (
                <FormItem className="space-y-3">
                  <div className="space-y-1">
                    <FormLabel required>{t("Groups")}</FormLabel>
                    <FormDescription>{t(FIELD_DESCRIPTIONS.GROUP)}</FormDescription>
                  </div>
                  <FormControl>
                    {isLoadingGroups ? (
                      <Skeleton className="h-10 w-full" />
                    ) : (
                      <MultiSelect
                        options={groupOptions}
                        selected={field.value}
                        onChange={field.onChange}
                        placeholder={t(FIELD_PLACEHOLDERS.GROUP)}
                      />
                    )}
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
        </div>
      </ChannelModelsSection>
    </div>
  );


  return { upstreamModelDetectionFields, modelMappingFields, modelsSection };
}
