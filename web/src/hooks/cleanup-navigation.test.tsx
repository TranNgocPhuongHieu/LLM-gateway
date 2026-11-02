// @vitest-environment jsdom
import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { NavGroup } from "@/components/layout/types";
import * as modelsApi from "@/features/models/api";
import * as profileApi from "@/features/profile/api";
import { normalizeUserSettings } from "@/features/profile/lib/user-settings";
import { useSidebarConfig, useIsSidebarModuleVisible } from "./use-sidebar-config";
import { useTopNavLinks } from "./use-top-nav-links";

const mocks = vi.hoisted(() => ({
  status: {} as Record<string, unknown>,
  user: { sidebar_modules: '{"personal":{"enabled":false}}', role: 1 },
}));
vi.mock("@/hooks/use-status", () => ({ useStatus: () => ({ status: mocks.status }) }));
vi.mock("@/lib/api", () => ({ api: {} }));
vi.mock("@/stores/auth-store", () => ({ useAuthStore: () => ({ auth: { user: mocks.user } }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

describe("lean navigation and notification settings", () => {
  it("retains model management without clients for retired deployments", () => {
    for (const name of ["getModels", "createModel", "updateModel", "deleteModel", "deleteModels", "getVendors", "syncUpstream"]) {
      expect(modelsApi).toHaveProperty(name, expect.any(Function));
    }
    expect(Object.keys(modelsApi).some((name) => /deployment|hardware|replicas|estimatePrice|clusterName/i.test(name))).toBe(false);
  });
  it("retains email binding and session security without retired social bindings", () => {
    for (const name of ["startEmailBinding", "resendEmailBinding", "bindEmail", "getLoginSessions", "revokeLoginSession", "changeAccountPassword"]) {
      expect(profileApi).toHaveProperty(name, expect.any(Function));
    }
    expect(profileApi).not.toHaveProperty("bindWeChat");
    expect(profileApi).not.toHaveProperty("startTelegramBind");
    expect(profileApi).not.toHaveProperty("sendEmailVerification");
  });
  it("uses owned documentation instead of legacy external Docs configuration", () => {
    mocks.status = { docs_link: "https://retired.invalid", HeaderNavModules: '{"docs":true}' };
    const { result } = renderHook(useTopNavLinks);
    expect(result.current.map((link) => link.href)).toEqual(["/", "/dashboard", "/pricing", "/docs"]);
  });
  it("ignores personal hiding while honoring the site-wide menu configuration", () => {
    mocks.status = { SidebarModulesAdmin: '{"console":{"enabled":true,"token":false}}' };
    const groups: NavGroup[] = [
      {
        title: "Menus",
        items: [
          { title: "Wallet", url: "/wallet" },
          { title: "Keys", url: "/keys" },
        ],
      },
    ];
    const { result } = renderHook(() => useSidebarConfig(groups));
    expect(result.current[0].items.map((item) => item.title)).toEqual(["Wallet"]);
    expect(renderHook(() => useIsSidebarModuleVisible("/wallet")).result.current).toBe(true);
    expect(renderHook(() => useIsSidebarModuleVisible("/keys")).result.current).toBe(false);
  });
  it("prepares email-only settings without resubmitting retired secrets", () => {
    const settings = normalizeUserSettings(
      JSON.stringify({
        notify_type: "gotify",
        gotify_token: "historical-fixture",
        quota_warning_threshold: 123,
        record_ip_log: true,
        notification_email: "alerts@example.invalid",
      }),
    );
    expect(settings.notify_type).toBe("email");
    expect(settings.quota_warning_threshold).toBe(123);
    expect(settings.notification_email).toBe("alerts@example.invalid");
    expect(settings.record_ip_log).toBe(true);
    expect(settings).not.toHaveProperty("gotify_token");
  });
});
