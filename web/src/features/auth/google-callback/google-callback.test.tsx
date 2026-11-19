// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StrictMode } from "react";

import { GoogleCallback, googleErrorMessage } from ".";

const mocks = vi.hoisted(() => ({
  navigate: vi.fn(),
  handleLoginResult: vi.fn(),
  finishGoogleLogin: vi.fn(),
  takeGoogleRedirect: vi.fn(),
  toastSuccess: vi.fn(),
}));

vi.mock("@tanstack/react-router", () => ({ useNavigate: () => mocks.navigate }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("sonner", () => ({ toast: { success: mocks.toastSuccess } }));
vi.mock("@/features/auth/api", () => ({
  finishGoogleLogin: mocks.finishGoogleLogin,
  takeGoogleRedirect: mocks.takeGoogleRedirect,
}));
vi.mock("@/features/auth/hooks/use-auth-redirect", () => ({
  useAuthRedirect: () => ({ handleLoginResult: mocks.handleLoginResult }),
}));

afterEach(cleanup);
beforeEach(() => {
  Object.values(mocks).forEach((mock) => mock.mockReset());
});

describe("Google callback page", () => {
  it("redeems the ticket exactly once and continues the shared login flow", async () => {
    mocks.finishGoogleLogin.mockResolvedValue({ success: true, data: { access_token: "a" } });
    mocks.takeGoogleRedirect.mockReturnValue("/keys");
    render(
      <StrictMode>
        <GoogleCallback search={{ ticket: "t-1" }} />
      </StrictMode>,
    );
    await waitFor(() => expect(mocks.handleLoginResult).toHaveBeenCalledTimes(1));
    expect(mocks.finishGoogleLogin).toHaveBeenCalledTimes(1);
    expect(mocks.finishGoogleLogin).toHaveBeenCalledWith("t-1");
    expect(mocks.handleLoginResult).toHaveBeenCalledWith({ access_token: "a" }, "/keys");
  });

  it("explains why an existing email cannot sign up with Google", () => {
    render(<GoogleCallback search={{ error: "email_exists" }} />);
    expect(screen.getByRole("alert").textContent).toBe(googleErrorMessage("email_exists"));
    expect(screen.getByRole("button", { name: "Back to sign in" })).toBeTruthy();
    expect(mocks.finishGoogleLogin).not.toHaveBeenCalled();
  });

  it("shows the server message when the ticket is rejected", async () => {
    mocks.finishGoogleLogin.mockResolvedValue({ success: false, message: "expired" });
    render(<GoogleCallback search={{ ticket: "t-2" }} />);
    expect(await screen.findByText("expired")).toBeTruthy();
    expect(mocks.handleLoginResult).not.toHaveBeenCalled();
  });

  it("returns to Security after linking Google", async () => {
    render(<GoogleCallback search={{ mode: "bind", status: "linked" }} />);
    await waitFor(() => expect(mocks.navigate).toHaveBeenCalledWith({ to: "/security", replace: true }));
    expect(mocks.toastSuccess).toHaveBeenCalled();
  });

  it("falls back to a generic message for unknown codes", () => {
    expect(googleErrorMessage("weird")).toBe(googleErrorMessage("server_error"));
  });
});
