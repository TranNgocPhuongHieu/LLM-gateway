// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { UserAuthForm } from "./user-auth-form";

const statusState = vi.hoisted(() => ({ status: {} as Record<string, unknown> }));

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}));
vi.mock("@/hooks/use-status", () => ({ useStatus: () => statusState }));
vi.mock("@/features/auth/hooks/use-auth-redirect", () => ({
  useAuthRedirect: () => ({ handleLoginResult: vi.fn() }),
}));
vi.mock("@/features/auth/hooks/use-turnstile", () => ({
  useTurnstile: () => ({
    isTurnstileEnabled: false,
    turnstileSiteKey: "",
    turnstileToken: "",
    setTurnstileToken: vi.fn(),
    validateTurnstile: () => true,
  }),
}));
vi.mock("@/features/auth/components/legal-consent", () => ({ LegalConsent: () => null }));

afterEach(cleanup);

describe("UserAuthForm", () => {
  it("puts Google sign-in below the password sign-in button", () => {
    statusState.status = { google_oauth_enabled: true };
    render(<UserAuthForm />);
    const buttons = screen.getAllByRole("button").map((b) => b.textContent?.trim());
    const signIn = buttons.indexOf("Sign in");
    const google = buttons.indexOf("Continue with Google");
    expect(signIn).toBeGreaterThanOrEqual(0);
    expect(google).toBeGreaterThan(signIn);
    expect(screen.getByText("or")).toBeTruthy();
  });

  it("shows Google alone when password login is disabled", () => {
    statusState.status = { google_oauth_enabled: true, password_login_enabled: false };
    render(<UserAuthForm />);
    expect(screen.getByRole("button", { name: "Continue with Google" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Sign in" })).toBeNull();
    expect(screen.queryByText("or")).toBeNull();
  });
});
