/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useNavigate } from "@tanstack/react-router";
import { Loader2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { finishGoogleLogin, takeGoogleRedirect } from "@/features/auth/api";
import { useAuthRedirect } from "@/features/auth/hooks/use-auth-redirect";
import { AuthOperationError } from "@/lib/secure-verification";
import { openAuthDialog } from "@/stores/auth-dialog-store";

export type GoogleCallbackSearch = {
  ticket?: string;
  error?: string;
  mode?: string;
  status?: string;
};

// Result codes set by GET /api/oauth/google/callback.
export const GOOGLE_ERROR_MESSAGES: Record<string, string> = {
  cancelled: "Google sign-in was cancelled.",
  invalid_state:
    "This Google sign-in link has expired or was opened in a different browser. Please try again.",
  exchange_failed: "Google sign-in could not be completed. Please try again.",
  email_unverified: "Your Google email address is not verified.",
  email_exists:
    "This email already belongs to an account. Sign in with your username and password, then link Google in Security.",
  registration_disabled: "New registrations are currently closed.",
  account_disabled: "This account has been disabled.",
  identity_taken: "This Google account is already linked to another account.",
  already_linked: "Google is already linked to this account.",
  session_changed: "Your session changed while linking Google. Please sign in and try again.",
  disabled: "Google sign-in is not enabled.",
  server_error: "Something went wrong. Please try again.",
};

export function googleErrorMessage(code: string | undefined): string {
  return GOOGLE_ERROR_MESSAGES[code ?? ""] ?? GOOGLE_ERROR_MESSAGES.server_error;
}

export function GoogleCallback({ search }: { search: GoogleCallbackSearch }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { handleLoginResult } = useAuthRedirect();
  const handled = useRef(false);
  const isBind = search.mode === "bind";
  const [error, setError] = useState<string | null>(
    search.error ? googleErrorMessage(search.error) : null,
  );

  useEffect(() => {
    // The ticket is single-use; StrictMode must not redeem it twice.
    if (handled.current) return;
    handled.current = true;

    if (isBind) {
      if (search.status === "linked") {
        toast.success(t("Google account linked."));
        void navigate({ to: "/security", replace: true });
      }
      return;
    }
    if (search.error || !search.ticket) {
      if (!search.error) setError(googleErrorMessage("invalid_state"));
      return;
    }
    const ticket = search.ticket;
    void (async () => {
      try {
        const res = await finishGoogleLogin(ticket);
        if (!res.success) {
          setError(res.message || googleErrorMessage("invalid_state"));
          return;
        }
        await handleLoginResult(res.data, takeGoogleRedirect() ?? "/dashboard");
      } catch (err: unknown) {
        setError(AuthOperationError.from(err, googleErrorMessage("server_error")).message);
      }
    })();
  }, [handleLoginResult, isBind, navigate, search.error, search.status, search.ticket, t]);

  if (!error) {
    return (
      <main className="flex min-h-[60vh] items-center justify-center gap-3 px-6 text-sm">
        <Loader2 className="animate-spin" aria-hidden="true" />
        {t("Signing you in with Google…")}
      </main>
    );
  }

  return (
    <main className="mx-auto flex min-h-[60vh] max-w-md flex-col items-center justify-center gap-5 px-6 text-center">
      <h1 className="text-xl font-semibold">
        {isBind ? t("Could not link Google") : t("Could not sign in with Google")}
      </h1>
      <p className="text-muted-foreground text-sm" role="alert">
        {t(error)}
      </p>
      {isBind ? (
        <Button onClick={() => void navigate({ to: "/security", replace: true })}>
          {t("Back to Security")}
        </Button>
      ) : (
        <Button
          onClick={() => {
            void navigate({ to: "/", replace: true });
            openAuthDialog("sign-in");
          }}
        >
          {t("Back to sign in")}
        </Button>
      )}
    </main>
  );
}
