import { describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { useSidebarData } from "./use-sidebar-data";

const mocks = vi.hoisted(() => ({ badge: vi.fn(() => "3") }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
vi.mock("@/features/topup-management/use-topup-review-badge", () => ({
  useTopupReviewBadge: mocks.badge,
}));

describe("useSidebarData", () => {
  it("labels the customer usage page as Usage history and keeps its URLs", () => {
    const { result } = renderHook(() => useSidebarData());
    const general = result.current.navGroups.find((group) => group.id === "general");
    const item = general?.items.find((entry) => entry.url === "/dashboard/models");
    expect(item?.title).toBe("Usage history");
    expect(item?.activeUrls).toEqual(["/dashboard/logs", "/dashboard/users"]);
    expect(general?.items.some((entry) => entry.title === "Dashboard")).toBe(false);
  });

  it("shows the manual-review count on the top-up management item", () => {
    const { result } = renderHook(() => useSidebarData());
    const admin = result.current.navGroups.find((group) => group.id === "admin");
    const topup = admin?.items.find((entry) => entry.url === "/topup-management");
    expect(topup?.badge).toBe("3");
    expect(mocks.badge).toHaveBeenCalled();
  });
});
