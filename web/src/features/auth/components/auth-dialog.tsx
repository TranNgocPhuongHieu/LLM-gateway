import { useEffect } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { Dialog } from "@/components/dialog";
import { Button } from "@/components/ui/button";
import { TermsFooter } from "@/features/auth/components/terms-footer";
import { OtpForm } from "@/features/auth/otp/components/otp-form";
import { UserAuthForm } from "@/features/auth/sign-in/components/user-auth-form";
import { SignUpForm } from "@/features/auth/sign-up/components/sign-up-form";
import { useStatus } from "@/hooks/use-status";
import { useAuthStore } from "@/stores/auth-store";

import {
  openAuthDialog,
  useAuthDialogState,
  type AuthDialogMode,
} from "@/stores/auth-dialog-store";

export function AuthEntryRoute(props: {
  mode: Exclude<AuthDialogMode, "verification">;
  redirectTo?: string;
}) {
  const navigate = useNavigate();

  useEffect(() => {
    openAuthDialog(props.mode, props.redirectTo);
    void navigate({ to: "/", replace: true });
  }, [navigate, props.mode, props.redirectTo]);

  return null;
}

export function AuthDialog() {
  const { t } = useTranslation();
  const { status } = useStatus();
  const mode = useAuthDialogState((state) => state.mode);
  const setMode = useAuthDialogState((state) => state.setMode);
  const close = useAuthDialogState((state) => state.close);
  const user = useAuthStore((state) => state.auth.user);
  const canRegister = !status?.self_use_mode_enabled && status?.register_enabled !== false;
  const isSignUp = mode === "sign-up" && canRegister;

  useEffect(() => {
    if (user && mode) close();
  }, [close, mode, user]);

  if (!mode || user) return null;
  if (mode === "verification") return <OtpForm />;

  return (
    <Dialog
      open
      onOpenChange={(open) => !open && close()}
      title={isSignUp ? t("Create an account") : t("Welcome back")}
      description={isSignUp ? undefined : t("Sign in to continue.")}
      contentClassName="auth-dialog w-full gap-5 rounded-[20px] border border-border bg-card p-6 text-card-foreground sm:max-w-[440px] sm:p-8 [&_[data-slot=dialog-close]]:size-11 [&_[data-slot=dialog-close]]:top-2 [&_[data-slot=dialog-close]]:right-2"
      titleClassName="pr-8 text-2xl font-bold"
      descriptionClassName="text-sm text-muted-foreground"
      contentHeight="auto"
      bodyClassName="overflow-y-auto"
    >
      <div className="space-y-5">
        {isSignUp ? (
          <SignUpForm />
        ) : (
          <UserAuthForm redirectTo="/dashboard" stayInPlace onNavigate={close} />
        )}

        {canRegister && (
          <p className="text-muted-foreground border-t border-border pt-5 text-center text-sm">
            {isSignUp ? t("Already have an account?") : t("Don't have an account?")}{" "}
            <Button
              type="button"
              variant="link"
              className="h-auto p-0 font-semibold"
              onClick={() => setMode(isSignUp ? "sign-in" : "sign-up")}
            >
              {isSignUp ? t("Sign in") : t("Sign up")}
            </Button>
          </p>
        )}

        <TermsFooter variant={isSignUp ? "sign-up" : "sign-in"} status={status} />
      </div>
    </Dialog>
  );
}
