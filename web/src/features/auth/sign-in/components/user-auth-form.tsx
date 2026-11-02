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
import { zodResolver } from "@hookform/resolvers/zod";
import { Link } from "@tanstack/react-router";
import { Loader2 } from "lucide-react";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import type { z } from "zod";
import { PasswordInput } from "@/components/password-input";
import { Turnstile } from "@/components/turnstile";
import { Button } from "@/components/ui/button";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { login } from "@/features/auth/api";
import { LegalConsent } from "@/features/auth/components/legal-consent";
import { loginFormSchema } from "@/features/auth/constants";
import { useAuthRedirect } from "@/features/auth/hooks/use-auth-redirect";
import { useTurnstile } from "@/features/auth/hooks/use-turnstile";
import type { AuthFormProps } from "@/features/auth/types";
import { useStatus } from "@/hooks/use-status";
import { handleServerError } from "@/lib/handle-server-error";
import { AuthOperationError } from "@/lib/secure-verification";
import { createServerError } from "@/lib/server-error-message";
import { cn } from "@/lib/utils";

export function UserAuthForm({
  className,
  redirectTo,
  stayInPlace = false,
  onNavigate,
  ...props
}: AuthFormProps) {
  const { t } = useTranslation();
  const [isLoading, setIsLoading] = useState(false);
  const [agreedToLegal, setAgreedToLegal] = useState(true);
  const [turnstileWidgetKey, setTurnstileWidgetKey] = useState(0);
  const { status } = useStatus();
  const requiresLegalConsent = false;
  const legalConsentErrorMessage = t("Please agree to the legal terms first");
  const loginFailedMessage = t("Login failed");
  const passwordLoginEnabled =
    (status?.password_login_enabled ?? status?.data?.password_login_enabled ?? true) !== false;
  const passwordLoginEncryptionEnabled =
    (status?.password_login_encryption_enabled ??
      status?.data?.password_login_encryption_enabled ??
      false) === true;
  const {
    isTurnstileEnabled,
    turnstileSiteKey,
    turnstileToken,
    setTurnstileToken,
    validateTurnstile,
  } = useTurnstile();
  const { handleLoginResult } = useAuthRedirect();
  const form = useForm<z.infer<typeof loginFormSchema>>({
    resolver: zodResolver(loginFormSchema),
    defaultValues: {
      username: "",
      password: "",
    },
  });

  async function onSubmit(data: z.infer<typeof loginFormSchema>) {
    if (requiresLegalConsent && !agreedToLegal) {
      toast.error(legalConsentErrorMessage);
      return;
    }

    if (!validateTurnstile()) return;

    const submittedTurnstileToken = turnstileToken;
    if (isTurnstileEnabled) {
      setTurnstileToken("");
      setTurnstileWidgetKey((current) => current + 1);
    }

    setIsLoading(true);
    try {
      const res = await login({
        username: data.username,
        password: data.password,
        turnstile: submittedTurnstileToken,
        passwordEncryptionEnabled: passwordLoginEncryptionEnabled,
      });

      if (res.success) {
        form.setValue("password", "");
        await handleLoginResult(res.data, redirectTo, stayInPlace);
      } else {
        handleServerError(createServerError(res, loginFailedMessage));
      }
    } catch (error: unknown) {
      handleServerError(AuthOperationError.from(error, loginFailedMessage));
    } finally {
      setIsLoading(false);
    }
  }

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        className={cn(
          "grid gap-5 [&_input]:h-12 [&_input]:rounded-xl [&_input]:bg-background [&_input]:px-3 [&_input]:focus-visible:border-primary [&_input]:focus-visible:ring-primary/30 [&_[data-slot=form-label]]:text-sm [&_[data-slot=form-label]]:font-medium [&_[data-slot=form-message]]:text-[13px]",
          className,
        )}
        {...props}
      >
        {passwordLoginEnabled && (
          <>
            {/* Username Field */}
            <FormField
              control={form.control}
              name="username"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("Username or Email")}</FormLabel>
                  <FormControl>
                    <Input
                      autoComplete="username"
                      placeholder={t("Enter your username or email")}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Password Field */}
            <FormField
              control={form.control}
              name="password"
              render={({ field }) => (
                <FormItem className="relative">
                  <FormLabel>{t("Password")}</FormLabel>
                  <FormControl>
                    <PasswordInput
                      autoComplete="current-password"
                      className="[&_input]:pr-12 [&_button]:size-11 [&_button]:end-0.5"
                      placeholder={t("Enter password")}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                  <Link
                    to="/forgot-password"
                    onClick={onNavigate}
                    className="text-primary absolute end-0 top-0 z-10 text-[13px] font-medium hover:underline"
                  >
                    {t("Forgot password?")}
                  </Link>
                </FormItem>
              )}
            />

            {isTurnstileEnabled && (
              <div className="min-h-[65px] w-full rounded-xl">
                <Turnstile
                  key={turnstileWidgetKey}
                  siteKey={turnstileSiteKey}
                  onVerify={setTurnstileToken}
                  onExpire={() => setTurnstileToken("")}
                />
              </div>
            )}

            {/* Submit Button */}
            <Button
              type="submit"
              className="h-12 w-full justify-center gap-2 rounded-xl font-semibold"
              disabled={isLoading || (requiresLegalConsent && !agreedToLegal)}
            >
              {isLoading && <Loader2 className="animate-spin" aria-hidden="true" />}
              {t("Sign in")}
            </Button>
          </>
        )}

        <LegalConsent
          status={status}
          checked={agreedToLegal}
          onCheckedChange={setAgreedToLegal}
          className="mt-1"
        />
      </form>
    </Form>
  );
}
