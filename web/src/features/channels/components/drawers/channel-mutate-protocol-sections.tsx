import type { TFunction } from "i18next";
import type { UseFormReturn } from "react-hook-form";

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";

import { type ChannelFormValues, HTTP_PROTOCOL_HTTP1 } from "../../lib";

type Props = {
  form: UseFormReturn<ChannelFormValues>;
  t: TFunction;
  sensitiveLocked: boolean;
  currentType: number;
  currentHttpProtocol: string | undefined;
};

export function ChannelProxyField({ form, t }: Props) {
  return <FormField control={form.control} name="proxy" render={({ field }) => (
    <FormItem><FormLabel>{t("Proxy Address")}</FormLabel><FormControl><Input placeholder={t("socks5://user:pass@host:port")} {...field} /></FormControl>
      <FormDescription>{t("Network proxy for this channel (supports HTTP, HTTPS, SOCKS5, and SOCKS5H)")}</FormDescription><FormMessage /></FormItem>
  )} />;
}

export function ChannelSystemPromptField({ form, t }: Props) {
  return <FormField control={form.control} name="system_prompt" render={({ field }) => (
    <FormItem><FormLabel>{t("System Prompt")}</FormLabel><FormControl><Textarea placeholder={t("Enter system prompt (user prompt takes priority)")} rows={3} {...field} /></FormControl>
      <FormDescription>{t("Default system prompt for this channel")}</FormDescription><FormMessage /></FormItem>
  )} />;
}

export function ChannelSystemPromptOverrideField({ form, t, sensitiveLocked }: Props) {
  return <FormField control={form.control} name="system_prompt_override" render={({ field }) => (
    <FormItem className="flex items-center justify-between"><div className="space-y-0.5"><FormLabel>{t("System Prompt Concatenation")}</FormLabel><FormDescription>{t("Concatenate channel system prompt with user&apos;s prompt")}</FormDescription></div>
      <FormControl><Switch disabled={sensitiveLocked} checked={field.value} onCheckedChange={field.onChange} /></FormControl></FormItem>
  )} />;
}

export function ChannelPassThroughField({ form, t, sensitiveLocked }: Props) {
  return <FormField control={form.control} name="pass_through_body_enabled" render={({ field }) => (
    <FormItem className="flex items-center justify-between px-4 py-3"><div className="space-y-0.5"><FormLabel>{t("Pass Through Body")}</FormLabel><FormDescription>{t("Pass request body directly to upstream")}</FormDescription></div>
      <FormControl><Switch disabled={sensitiveLocked} checked={field.value} onCheckedChange={field.onChange} /></FormControl></FormItem>
  )} />;
}

export function ChannelThinkingField({ form, t, sensitiveLocked }: Props) {
  return <FormField control={form.control} name="thinking_to_content" render={({ field }) => (
    <FormItem className="flex items-center justify-between px-4 py-3"><div className="space-y-0.5"><FormLabel>{t("Thinking to Content")}</FormLabel><FormDescription>{t("Convert reasoning_content to <think> tag in content")}</FormDescription></div>
      <FormControl><Switch disabled={sensitiveLocked} checked={field.value} onCheckedChange={field.onChange} /></FormControl></FormItem>
  )} />;
}

export function ChannelFormatField({ form, t, sensitiveLocked, currentType }: Props) {
  if (currentType !== 1) return null;
  return <FormField control={form.control} name="force_format" render={({ field }) => (
    <FormItem className="flex items-center justify-between px-4 py-3"><div className="space-y-0.5"><FormLabel>{t("Force Format")}</FormLabel><FormDescription>{t("Force format response to OpenAI standard (OpenAI channel only)")}</FormDescription></div>
      <FormControl><Switch disabled={sensitiveLocked} checked={field.value} onCheckedChange={field.onChange} /></FormControl></FormItem>
  )} />;
}

export function ChannelHttpProtocolField({ form, t, sensitiveLocked }: Props) {
  return <FormField control={form.control} name="http_protocol" render={({ field }) => (
    <FormItem><FormLabel>{t("HTTP Protocol")}</FormLabel><Select disabled={sensitiveLocked} items={[{ value: "auto", label: t("Auto") }, { value: HTTP_PROTOCOL_HTTP1, label: t("HTTP/1.1") }]} value={field.value || "auto"} onValueChange={(value) => { const next = value === HTTP_PROTOCOL_HTTP1 ? HTTP_PROTOCOL_HTTP1 : "auto"; field.onChange(next); if (next === HTTP_PROTOCOL_HTTP1) form.setValue("http2_connection_shards", 1, { shouldDirty: true, shouldValidate: true }); }}>
      <FormControl><SelectTrigger><SelectValue /></SelectTrigger></FormControl><SelectContent alignItemWithTrigger={false}><SelectGroup><SelectItem value="auto">{t("Auto")}</SelectItem><SelectItem value={HTTP_PROTOCOL_HTTP1}>{t("HTTP/1.1")}</SelectItem></SelectGroup></SelectContent>
    </Select><FormDescription>{t("Auto negotiates HTTP/2 when available. HTTP/1.1 forces multiple keep-alive connections under concurrency.")}</FormDescription><FormMessage /></FormItem>
  )} />;
}

export function ChannelHttpShardsField({ form, t, sensitiveLocked, currentHttpProtocol }: Props) {
  const http1Selected = currentHttpProtocol === HTTP_PROTOCOL_HTTP1;
  const shardItems = Array.from({ length: 8 }, (_, index) => ({ value: String(index + 1), label: String(index + 1) }));
  return <FormField control={form.control} name="http2_connection_shards" render={({ field }) => (
    <FormItem><FormLabel>{t("HTTP/2 Connection Shards")}</FormLabel><Select items={shardItems} value={String(field.value || 1)} disabled={sensitiveLocked || http1Selected} onValueChange={(value) => field.onChange(Number(value))}>
      <FormControl><SelectTrigger disabled={sensitiveLocked || http1Selected}><SelectValue /></SelectTrigger></FormControl><SelectContent alignItemWithTrigger={false}><SelectGroup>{shardItems.map((item) => <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>)}</SelectGroup></SelectContent>
    </Select><FormDescription>{http1Selected ? t("HTTP/2 connection shards are unavailable when HTTP/1.1 is selected.") : t("Spread HTTP/2 traffic across multiple reusable connections to the same upstream origin (1-8).")}</FormDescription><FormMessage /></FormItem>
  )} />;
}
