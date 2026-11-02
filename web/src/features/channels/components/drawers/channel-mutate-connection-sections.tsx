import type { TFunction } from "i18next";
import type { ReactNode } from "react";
import type { UseFormReturn } from "react-hook-form";
import { Code, Copy, Eye, Loader2, SlidersHorizontal, Trash2, Wand2 } from "lucide-react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { FormControl, FormDescription, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { JsonCodeEditor } from "@/components/json-code-editor";
import { JsonEditor } from "@/components/json-editor";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { sideDrawerSwitchItemClassName } from "@/components/drawer-layout";
import { ChannelApiAccessSection, ChannelAuthSection, ChannelBasicSection } from "./sections";
import type { ChannelFormValues } from "../../lib";
import {
  CLAUDE_FIELD_PASSTHROUGH_TYPES,
  CHANNEL_TYPE_WARNINGS,
  FIELD_PASSTHROUGH_TYPES,
  OPENAI_FIELD_PASSTHROUGH_TYPES,
} from "../../constants";

export type ChannelConnectionSectionsProps = {
  form: UseFormReturn<ChannelFormValues>;
  currentType: number;
  t: TFunction;
  [key: string]: any;
};

export function buildChannelConnectionSections(props: ChannelConnectionSectionsProps): Record<string, ReactNode> {
  const { form, currentType, t } = props;
  const { sensitiveLocked, isEditing, configuration, channelConfigurationBlockClassName, CardHeading,
    FIELD_PLACEHOLDERS, FIELD_DESCRIPTIONS, isSubmitting, setParamOverrideEditorOpen,
    isBatchMode, keyMode, isMultiKeyChannel, canRevealChannelKey, handleDeduplicateKeys,
    handleRevealKey, isChannelKeyLoading, verification, channelKey, copyToClipboard, multiKeyMode,
    multiKeyType, addModeOptions, SubHeading, getKeyPromptForType } = props;

  const basicSection = (
    <div className="scroll-mt-4">
      <ChannelBasicSection>
        <div className="grid gap-4">
          <FormField
            control={form.control}
            name="name"
            render={({ field }) => (
              <FormItem>
                <FormLabel required>{t("Name")}</FormLabel>
                <FormControl>
                  <Input placeholder={t(FIELD_PLACEHOLDERS.NAME)} {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </div>

        {!isEditing && (
          <FormField
            control={form.control}
            name="status"
            render={({ field }) => (
              <FormItem className={sideDrawerSwitchItemClassName()}>
                <div className="flex flex-col gap-0.5">
                  <FormLabel>{t("Enabled")}</FormLabel>
                  <FormDescription className="text-xs">
                    {t("Enable or disable this channel")}
                  </FormDescription>
                </div>
                <FormControl>
                  <Switch
                    checked={field.value === 1}
                    onCheckedChange={(checked) => field.onChange(checked ? 1 : 2)}
                  />
                </FormControl>
              </FormItem>
            )}
          />
        )}

        {currentType === 1 && (
          <fieldset disabled={sensitiveLocked} className="disabled:opacity-60">
            <FormField
              control={form.control}
              name="openai_organization"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("OpenAI Organization")}</FormLabel>
                  <FormControl>
                    <Input placeholder={t("org-...")} {...field} />
                  </FormControl>
                  <FormDescription>
                    {sensitiveLocked
                      ? t("No permission to perform this action")
                      : t(FIELD_DESCRIPTIONS.OPENAI_ORG)}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </fieldset>
        )}
      </ChannelBasicSection>
    </div>
  );

  const overrideFields = (
    <div
      role="group"
      aria-label={t("Override Rules")}
      className={channelConfigurationBlockClassName(
        configuration.blocks.overrideRules,
        "flex flex-col gap-4",
      )}
    >
      <SubHeading
        title={t("Override Rules")}
        status={configuration.blocks.overrideRules}
        icon={<Code className="h-3.5 w-3.5" />}
        iconTone="chart-4"
      />

      <FormField
        control={form.control}
        name="status_code_mapping"
        render={({ field }) => (
          <FormItem className="space-y-3">
            <div className="space-y-1">
              <FormLabel>{t("Status Code Mapping")}</FormLabel>
              <FormDescription>{t("Map upstream status codes to different codes")}</FormDescription>
            </div>
            <FormControl>
              <JsonEditor
                value={field.value || ""}
                onChange={field.onChange}
                disabled={isSubmitting}
                keyPlaceholder="400"
                valuePlaceholder="500"
                keyLabel="Original Code"
                valueLabel="Mapped Code"
                emptyMessage={t("No status code mappings configured.")}
                template={{ "400": "500", "429": "503" }}
                valueType="string"
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      {sensitiveLocked && (
        <p className="text-muted-foreground text-xs">{t("No permission to perform this action")}</p>
      )}
      <fieldset disabled={sensitiveLocked} className="space-y-4 disabled:opacity-60">
        <FormField
          control={form.control}
          name="param_override"
          render={({ field }) => (
            <FormItem className="space-y-3 border-t pt-4">
              <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
                <div className="space-y-1">
                  <FormLabel>{t("Parameter Override")}</FormLabel>
                  <FormDescription>
                    {t("Override request parameters. Cannot override stream parameter.")}
                  </FormDescription>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => setParamOverrideEditorOpen(true)}
                  >
                    <Wand2 className="mr-2 h-4 w-4" />
                    {t("Visual edit")}
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      field.onChange(
                        JSON.stringify(
                          {
                            operations: [
                              {
                                path: "temperature",
                                mode: "set",
                                value: 0.7,
                                conditions: [
                                  {
                                    path: "model",
                                    mode: "prefix",
                                    value: "gpt",
                                  },
                                ],
                                logic: "AND",
                              },
                            ],
                          },
                          null,
                          2,
                        ),
                      );
                    }}
                  >
                    <Code className="mr-2 h-4 w-4" />
                    {t("New Format Template")}
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => field.onChange("")}
                  >
                    {t("Clear")}
                  </Button>
                </div>
              </div>
              <FormControl>
                <JsonCodeEditor
                  value={field.value || ""}
                  onChange={field.onChange}
                  name={field.name}
                  onBlur={field.onBlur}
                  textareaRef={field.ref}
                  disabled={sensitiveLocked || isSubmitting}
                  placeholder={t("Override request parameters. Cannot override stream parameter.")}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name="header_override"
          render={({ field }) => (
            <FormItem className="space-y-3 border-t pt-4">
              <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
                <div className="space-y-1">
                  <FormLabel>{t("Request Header Override")}</FormLabel>
                  <FormDescription>{t("Override request headers")}</FormDescription>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() =>
                      field.onChange(
                        JSON.stringify(
                          {
                            "*": true,
                            "re:^X-Trace-.*$": true,
                            "X-Foo": "{client_header:X-Foo}",
                            Authorization: "Bearer {api_key}",
                          },
                          null,
                          2,
                        ),
                      )
                    }
                  >
                    {t("Fill Template")}
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => field.onChange(JSON.stringify({ "*": true }, null, 2))}
                  >
                    {t("Passthrough Template")}
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => field.onChange("")}
                  >
                    {t("Clear")}
                  </Button>
                </div>
              </div>
              <FormControl>
                <JsonCodeEditor
                  value={field.value || ""}
                  onChange={field.onChange}
                  name={field.name}
                  onBlur={field.onBlur}
                  textareaRef={field.ref}
                  disabled={sensitiveLocked || isSubmitting}
                  placeholder={t("Enter JSON to override request headers")}
                  heightClassName="h-40 min-h-40 max-h-40"
                />
              </FormControl>
              <FormDescription className="text-xs">
                {t("Supported variables")}:{" "}
                <code className="bg-muted rounded px-1 py-0.5">{"{api_key}"}</code> —{" "}
                {t("Channel key")},{" "}
                <code className="bg-muted rounded px-1 py-0.5">{"{client_header:NAME}"}</code> —{" "}
                {t("Client header value")}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      </fieldset>
    </div>
  );

  const fieldPassthroughFields = FIELD_PASSTHROUGH_TYPES.has(currentType) && (
    <div
      role="group"
      aria-label={t("Field passthrough controls")}
      className={channelConfigurationBlockClassName(
        configuration.blocks.fieldPassthrough,
        "flex flex-col gap-4",
      )}
    >
      <CardHeading
        title={t("Field passthrough controls")}
        status={configuration.blocks.fieldPassthrough}
        icon={<SlidersHorizontal className="h-4 w-4" />}
        iconTone="chart-4"
      />
      <fieldset disabled={sensitiveLocked} className="disabled:opacity-60">
        <div className="divide-border space-y-0 divide-y border-y">
          <FormField
            control={form.control}
            name="allow_service_tier"
            render={({ field }) => (
              <FormItem className="flex items-center justify-between gap-3 px-4 py-3">
                <div className="space-y-0.5">
                  <FormLabel className="text-sm">{t("Allow service_tier passthrough")}</FormLabel>
                  <FormDescription>{t("Pass through the service_tier field")}</FormDescription>
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

          {OPENAI_FIELD_PASSTHROUGH_TYPES.has(currentType) && (
            <>
              <FormField
                control={form.control}
                name="disable_store"
                render={({ field }) => (
                  <FormItem className="flex items-center justify-between gap-3 px-4 py-3">
                    <div className="space-y-0.5">
                      <FormLabel className="text-sm">{t("Disable store passthrough")}</FormLabel>
                      <FormDescription>
                        {t("When enabled, the store field will be blocked")}
                      </FormDescription>
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
                name="allow_safety_identifier"
                render={({ field }) => (
                  <FormItem className="flex items-center justify-between gap-3 px-4 py-3">
                    <div className="space-y-0.5">
                      <FormLabel className="text-sm">
                        {t("Allow safety_identifier passthrough")}
                      </FormLabel>
                      <FormDescription>
                        {t("Pass through the safety_identifier field")}
                      </FormDescription>
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
                name="allow_include_obfuscation"
                render={({ field }) => (
                  <FormItem className="flex items-center justify-between gap-3 px-4 py-3">
                    <div className="space-y-0.5">
                      <FormLabel className="text-sm">
                        {t("Allow include usage obfuscation passthrough")}
                      </FormLabel>
                      <FormDescription>
                        {t("Pass through the include field for usage obfuscation")}
                      </FormDescription>
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
                name="allow_inference_geo"
                render={({ field }) => (
                  <FormItem className="flex items-center justify-between gap-3 px-4 py-3">
                    <div className="space-y-0.5">
                      <FormLabel className="text-sm">
                        {t("Allow inference geography passthrough")}
                      </FormLabel>
                      <FormDescription>
                        {t("Pass through the inference_geo field for geographic routing")}
                      </FormDescription>
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
            </>
          )}

          {CLAUDE_FIELD_PASSTHROUGH_TYPES.has(currentType) && (
            <>
              <FormField
                control={form.control}
                name="allow_speed"
                render={({ field }) => (
                  <FormItem className="flex items-center justify-between gap-3 px-4 py-3">
                    <div className="space-y-0.5">
                      <FormLabel className="text-sm">{t("Allow speed passthrough")}</FormLabel>
                      <FormDescription>
                        {t("Pass through the speed field for Claude inference speed mode control")}
                      </FormDescription>
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
            </>
          )}
        </div>
      </fieldset>
    </div>
  );

  const connectionSection = (
    <div className="scroll-mt-4">
      <ChannelApiAccessSection>
        {CHANNEL_TYPE_WARNINGS[currentType] && (
          <Alert>
            <AlertDescription>{t(CHANNEL_TYPE_WARNINGS[currentType])}</AlertDescription>
          </Alert>
        )}

        {sensitiveLocked && (
          <Alert className="border-amber-200 bg-amber-50 text-amber-900 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-50">
            <AlertDescription>{t("No permission to perform this action")}</AlertDescription>
          </Alert>
        )}

        <div className="border-border/60 bg-muted/10 rounded-lg border p-4">
          <fieldset disabled={sensitiveLocked} className="space-y-4 disabled:opacity-60">
            {![3, 8, 22, 36, 45].includes(currentType) && (
              <FormField
                control={form.control}
                name="base_url"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("Base URL")}</FormLabel>
                    <FormControl>
                      <Input placeholder={t(FIELD_PLACEHOLDERS.BASE_URL)} {...field} />
                    </FormControl>
                    <FormDescription>
                      {t(
                        "Custom API base URL. For official channels, New API has built-in addresses. Only fill this for third-party proxy sites or special endpoints. Do not add /v1 or trailing slash.",
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            <ChannelAuthSection>
              {!isEditing && (
                <FormField
                  control={form.control}
                  name="multi_key_mode"
                  render={({ field }) => (
                    <FormItem className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                      <FormLabel className="text-muted-foreground text-xs font-medium">
                        {t("Add Mode")}
                      </FormLabel>
                      <Select
                        disabled={sensitiveLocked}
                        items={addModeOptions.map((option: { value: string; label: string }) => ({
                          value: option.value,
                          label: t(option.label),
                        }))}
                        onValueChange={field.onChange}
                        value={field.value}
                      >
                        <FormControl>
                          <SelectTrigger size="sm" className="w-full sm:w-56">
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent alignItemWithTrigger={false}>
                          <SelectGroup>
                            {addModeOptions.map((option: { value: string; label: string }) => (
                              <SelectItem key={option.value} value={option.value}>
                                {t(option.label)}
                              </SelectItem>
                            ))}
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}

              <FormField
                control={form.control}
                name="key"
                render={({ field }) => {
                  let keyPlaceholder = t(getKeyPromptForType(currentType));
                  if (isEditing) {
                    keyPlaceholder = t("Leave empty to keep existing key");
                  } else if (isBatchMode) {
                    keyPlaceholder = t("Enter one key per line for batch creation");
                  }

                  let keyDescription: ReactNode = t(FIELD_DESCRIPTIONS.KEY);
                  if (isEditing) {
                    let keyModeDescription = t(
                      "Append mode: New keys will be added to the end of the existing key list",
                    );
                    if (keyMode === "replace") {
                      keyModeDescription = t(
                        "Replace mode: Will completely replace all existing keys",
                      );
                    }
                    keyDescription = (
                      <>
                        {t("Enter new key to update, or leave empty to keep current key")}
                        {isMultiKeyChannel && (
                          <span className="text-warning mt-1 block">{keyModeDescription}</span>
                        )}
                      </>
                    );
                  } else if (isBatchMode) {
                    keyDescription = t("Enter one API key per line for batch creation");
                  }
                  return (
                    <FormItem>
                      <FormLabel required>{t("API Key")}</FormLabel>
                      <FormControl>
                        <Textarea
                          placeholder={keyPlaceholder}
                          rows={isBatchMode ? 8 : 4}
                          {...field}
                        />
                      </FormControl>
                      <FormDescription>
                        <span className="flex flex-col gap-2">
                          <span>{keyDescription}</span>
                          {!isEditing && isBatchMode && (
                            <Button
                              type="button"
                              variant="outline"
                              size="sm"
                              onClick={handleDeduplicateKeys}
                              className="w-fit"
                            >
                              <Trash2 className="mr-2 h-4 w-4" />
                              {t("Remove Duplicates")}
                            </Button>
                          )}
                        </span>
                      </FormDescription>
                      {isEditing && canRevealChannelKey && (
                        <div className="border-border/60 mt-4 flex flex-col gap-3 border-y border-dashed py-4">
                          <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                            <div>
                              <p className="text-sm font-medium">{t("Current key")}</p>
                              <p className="text-muted-foreground text-xs">
                                {t("Verification required to reveal the saved key.")}
                              </p>
                            </div>
                            <div className="flex items-center gap-2">
                              <Button
                                type="button"
                                variant="outline"
                                size="sm"
                                onClick={handleRevealKey}
                                disabled={isChannelKeyLoading || verification.isActive}
                              >
                                {isChannelKeyLoading || verification.isActive ? (
                                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                                ) : (
                                  <Eye className="mr-2 h-4 w-4" />
                                )}
                                {t("Reveal key")}
                              </Button>
                              <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                onClick={async () => {
                                  if (channelKey) {
                                    await copyToClipboard(channelKey);
                                  }
                                }}
                                disabled={!channelKey}
                              >
                                <Copy className="mr-2 h-4 w-4" />
                                {t("Copy")}
                              </Button>
                            </div>
                          </div>
                          <Input
                            readOnly
                            value={channelKey ?? ""}
                            placeholder={t("Hidden — verify to reveal")}
                            className="font-mono"
                          />
                        </div>
                      )}
                      <FormMessage />
                    </FormItem>
                  );
                }}
              />

              {isEditing && isMultiKeyChannel && (
                <FormField
                  control={form.control}
                  name="key_mode"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t("Key Update Mode")}</FormLabel>
                      <Select
                        disabled={sensitiveLocked}
                        items={[
                          {
                            value: "append",
                            label: t("Append to existing keys"),
                          },
                          {
                            value: "replace",
                            label: t("Replace all existing keys"),
                          },
                        ]}
                        onValueChange={field.onChange}
                        value={field.value}
                      >
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent alignItemWithTrigger={false}>
                          <SelectGroup>
                            <SelectItem value="append">{t("Append to existing keys")}</SelectItem>
                            <SelectItem value="replace">
                              {t("Replace all existing keys")}
                            </SelectItem>
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                      <FormDescription>
                        {field.value === "replace"
                          ? t("Replace mode: Will completely replace all existing keys")
                          : t(
                              "Append mode: New keys will be added to the end of the existing key list",
                            )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}

              {(isMultiKeyChannel || (!isEditing && multiKeyMode === "multi_to_single")) && (
                <FormField
                  control={form.control}
                  name="multi_key_type"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t("Multi-Key Strategy")}</FormLabel>
                      <Select
                        disabled={sensitiveLocked}
                        items={[
                          {
                            value: "random",
                            label: t("Random"),
                          },
                          {
                            value: "polling",
                            label: t("Polling"),
                          },
                        ]}
                        onValueChange={field.onChange}
                        value={field.value}
                      >
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent alignItemWithTrigger={false}>
                          <SelectGroup>
                            <SelectItem value="random">{t("Random")}</SelectItem>
                            <SelectItem value="polling">{t("Polling")}</SelectItem>
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                      <FormDescription>
                        {multiKeyType === "polling" ? (
                          <span className="text-warning">
                            {t(
                              "Polling mode requires Redis and memory cache, otherwise performance will be significantly degraded",
                            )}
                          </span>
                        ) : (
                          t("Randomly select a key from the pool for each request")
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
            </ChannelAuthSection>
          </fieldset>
        </div>
      </ChannelApiAccessSection>
    </div>
  );


  return { basicSection, overrideFields, fieldPassthroughFields, connectionSection };
}
