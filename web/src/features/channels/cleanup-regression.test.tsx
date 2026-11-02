import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { ChannelProviderPicker } from "./components/drawers/channel-provider-picker";
import { CHANNEL_TYPE_OPTIONS, MODEL_FETCHABLE_TYPES } from "./constants";
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformFormDataToCreatePayload,
} from "./lib/channel-form";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
afterEach(cleanup);

describe("gateway-only channel configuration", () => {
  it("enables model discovery for both supported protocols only", () => {
    expect([...MODEL_FETCHABLE_TYPES]).toEqual([1, 14]);
    expect(MODEL_FETCHABLE_TYPES.has(0)).toBe(false);
    expect(MODEL_FETCHABLE_TYPES.has(24)).toBe(false);
  });
  const form = {
    ...CHANNEL_FORM_DEFAULT_VALUES,
    type: 1,
    name: "Fixture",
    key: "fixture-not-a-real-secret",
    models: "fixture-model",
  };

  it("offers the supported OpenAI and Anthropic upstream protocols", () => {
    const onSelect = vi.fn();
    render(<ChannelProviderPicker disabled={false} onSelect={onSelect} />);
    expect(CHANNEL_TYPE_OPTIONS.map(({ value }) => value)).toEqual([1, 14]);
    fireEvent.click(screen.getByRole("button", { name: "OpenAI Compatible" }));
    expect(onSelect).toHaveBeenCalledWith({ kind: "builtin", type: 1 });
    fireEvent.click(screen.getByRole("button", { name: "Anthropic Messages" }));
    expect(onSelect).toHaveBeenCalledWith({ kind: "builtin", type: 14 });
  });

  it("preserves priority and weight editing while removing quota checks from Channels", () => {
    const read = (path: string) => readFileSync(fileURLToPath(new URL(path, import.meta.url)), "utf8");
    const columns = read("./components/channels-columns.tsx");
    const actions = read("./components/data-table-row-actions.tsx");
    const primary = read("./components/channels-primary-buttons.tsx");
    expect(columns).toContain('field="priority"');
    expect(columns).toContain('field="weight"');
    expect(columns).not.toContain("updateChannelBalance");
    expect(actions).not.toContain("Query Balance");
    expect(primary).not.toContain("handleUpdateAllBalances");
  });

  it("does not permit provider selection when sensitive editing is denied", () => {
    const onSelect = vi.fn();
    render(<ChannelProviderPicker disabled onSelect={onSelect} />);
    fireEvent.click(screen.getByRole("button", { name: "OpenAI Compatible" }));
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("validates supported routing and rejects retired providers", () => {
    expect(channelFormSchema.safeParse(form).success).toBe(true);
    for (const type of [0, 2, 3, 4, 20, 24, 33, 41, 57, 58, 60, 61]) {
      expect(channelFormSchema.safeParse({ ...form, type }).success).toBe(false);
    }
  });

  it("preserves routing, custom headers, HTTP transport and existing generic settings", () => {
    const payload = transformFormDataToCreatePayload({
      ...form,
      base_url: "https://fixture.invalid/",
      model_mapping: '{"fixture-model":"upstream-model"}',
      header_override: '{"X-Fixture":"retained"}',
      http_protocol: "http1",
      settings: '{"existing_fixture_flag":true}',
      allow_service_tier: true,
    });
    expect(payload.channel.type).toBe(1);
    expect(payload.channel.base_url).toBe("https://fixture.invalid");
    expect(payload.channel.model_mapping).toBe('{"fixture-model":"upstream-model"}');
    expect(payload.channel.header_override).toBe('{"X-Fixture":"retained"}');
    expect(JSON.parse(payload.channel.setting ?? "{}").http_protocol).toBe("http1");
    const settings = JSON.parse(payload.channel.settings ?? "{}");
    expect(settings.existing_fixture_flag).toBe(true);
    expect(settings.allow_service_tier).toBe(true);
    expect(JSON.parse(payload.channel.setting ?? "{}").task_plugin_key).toBeUndefined();
  });
});
