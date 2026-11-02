// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AuthDialog, AuthEntryRoute } from "../components/auth-dialog";
import { openAuthDialog, useAuthDialogState } from "@/stores/auth-dialog-store";
import { useAuthStore } from "@/stores/auth-store";

const { navigateMock } = vi.hoisted(() => ({ navigateMock: vi.fn() }));

vi.mock("@tanstack/react-router", () => ({
  useNavigate: () => navigateMock,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("@/hooks/use-status", () => ({
  useStatus: () => ({ status: { register_enabled: true } }),
}));

vi.mock("@/features/auth/sign-in/components/user-auth-form", () => ({
  UserAuthForm: (props: { redirectTo?: string }) => (
    <div data-testid="sign-in-form" data-redirect={props.redirectTo}>
      Sign-in form
    </div>
  ),
}));

vi.mock("@/features/auth/sign-up/components/sign-up-form", () => ({
  SignUpForm: () => <div>Sign-up form</div>,
}));

vi.mock("@/features/auth/otp/components/otp-form", () => ({
  OtpForm: () => <div>Verification form</div>,
}));

afterEach(cleanup);

beforeEach(() => {
  useAuthDialogState.getState().close();
  useAuthStore.getState().auth.reset();
});

describe("authentication dialog", () => {
  it("opens on the current view, switches between forms, and closes with Escape", async () => {
    const user = userEvent.setup();
    render(<AuthDialog />);

    openAuthDialog("sign-in");
    expect(await screen.findByRole("dialog", { name: "Welcome back" })).toBeTruthy();
    expect(screen.getByText("Sign in to continue.")).toBeTruthy();
    expect(screen.getByTestId("sign-in-form").getAttribute("data-redirect")).toBe("/dashboard");
    expect(screen.getByText("Sign-in form")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Sign up" }));
    expect(await screen.findByRole("dialog", { name: "Create an account" })).toBeTruthy();
    expect(screen.getByText("Sign-up form")).toBeTruthy();

    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(useAuthDialogState.getState().mode).toBeNull();
  });

  it("renders the existing verification flow as the modal view for login challenges", () => {
    useAuthDialogState.getState().open("verification");
    render(<AuthDialog />);

    expect(screen.getByText("Verification form")).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("opens the sign-in modal for a direct auth route and returns to its safe destination", async () => {
    render(
      <>
        <AuthEntryRoute mode="sign-in" redirectTo="/pricing?group=team" />
        <AuthDialog />
      </>,
    );

    expect(await screen.findByRole("dialog", { name: "Welcome back" })).toBeTruthy();
    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith({ to: "/", replace: true });
    });
    expect(useAuthDialogState.getState().redirectTo).toBe("/pricing?group=team");
  });
});
