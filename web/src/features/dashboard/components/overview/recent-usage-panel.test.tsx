import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { RecentUsagePanel } from "./recent-usage-panel";

const mocks = vi.hoisted(() => ({
  user: { id: 1, role: 1, username: "customer" },
  setUser: vi.fn(),
  all: vi.fn(),
  self: vi.fn(),
  balance: vi.fn(),
  models: vi.fn(),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children: React.ReactNode }) => <span>{children}</span>,
}));
vi.mock("@/lib/format", () => ({ formatLogQuota: String, formatTimestampToDate: String }));
vi.mock("@/features/usage-logs/api", () => ({ getAllLogs: mocks.all, getUserLogs: mocks.self }));
vi.mock("@/lib/api", () => ({ getSelf: mocks.balance, getUserModels: mocks.models }));
vi.mock("@/stores/auth-store", () => {
  const state = () => ({
    auth: { user: mocks.user, session: { sid: "test" }, setUser: mocks.setUser },
  });
  return {
    useAuthStore: Object.assign(
      (selector: (value: ReturnType<typeof state>) => unknown) => selector(state()),
      { getState: state },
    ),
  };
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function mount(role = 1, balanceUserId = 1) {
  mocks.user.role = role;
  const response = {
    success: true,
    data: {
      items: [
        {
          id: 1,
          user_id: 1,
          created_at: 1,
          type: 2,
          content: "",
          model_name: "model-test",
          channel_name: "private-channel",
        },
      ],
    },
  };
  mocks.self.mockResolvedValue(response);
  mocks.all.mockResolvedValue(response);
  mocks.models.mockResolvedValue({ success: true, data: ["model-test", "other-model"] });
  mocks.balance.mockResolvedValue({
    success: true,
    data: { id: balanceUserId, quota: 900, used_quota: 100, request_count: 1 },
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  render(
    <QueryClientProvider client={client}>
      <RecentUsagePanel />
    </QueryClientProvider>,
  );
}

describe("dashboard usage and settled balance", () => {
  it("exposes model selection and bounded refresh choices", async () => {
    mount();
    expect(await screen.findByRole("combobox", { name: "Model" })).toBeTruthy();
    const refresh = screen.getByRole("combobox", { name: "Refresh interval" });
    expect((refresh as HTMLSelectElement).value).toBe("30000");
    expect(Array.from(refresh.querySelectorAll("option")).map((option) => option.value)).toEqual([
      "30000",
      "300000",
      "900000",
    ]);
  });

  it("uses customer logs, hides channels and refreshes balance from server", async () => {
    mount();
    expect((await screen.findAllByText("model-test")).length).toBeGreaterThan(0);
    expect(screen.queryByText("Channel")).toBeNull();
    expect(screen.queryByText("private-channel")).toBeNull();
    expect(mocks.all).not.toHaveBeenCalled();
    await waitFor(() =>
      expect(mocks.setUser).toHaveBeenCalledWith(
        expect.objectContaining({ quota: 900, used_quota: 100 }),
      ),
    );
  });

  it("allows only the owner role to see channel and global logs", async () => {
    mount(100);
    expect((await screen.findAllByText("model-test")).length).toBeGreaterThan(0);
    expect(mocks.all).toHaveBeenCalledWith({ p: 1, page_size: 10 });
    expect(mocks.self).not.toHaveBeenCalled();
    expect(screen.queryByText("private-channel")).toBeNull();
  });

  it("keeps non-root administrators on their own private log endpoint", async () => {
    mount(10);
    expect((await screen.findAllByText("model-test")).length).toBeGreaterThan(0);
    expect(mocks.self).toHaveBeenCalledWith({ p: 1, page_size: 10 });
    expect(mocks.all).not.toHaveBeenCalled();
    expect(screen.queryByText("Channel")).toBeNull();
  });

  it("does not overwrite balance when the server returns another account", async () => {
    mount(1, 2);
    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(mocks.setUser).not.toHaveBeenCalled();
  });
});
