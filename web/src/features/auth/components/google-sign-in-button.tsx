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
import { Loader2 } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { rememberGoogleRedirect, startGoogleLogin } from "@/features/auth/api";
import { cn } from "@/lib/utils";

export function GoogleIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 48 48" aria-hidden="true" className={cn("size-5", className)}>
      <path
        fill="#FFC107"
        d="M43.6 20.5H42V20H24v8h11.3C33.7 32.7 29.2 36 24 36c-6.6 0-12-5.4-12-12s5.4-12 12-12c3.1 0 5.8 1.2 7.9 3.1l5.7-5.7C34 6.1 29.3 4 24 4 12.9 4 4 12.9 4 24s8.9 20 20 20 20-8.9 20-20c0-1.2-.1-2.3-.4-3.5z"
      />
      <path
        fill="#FF3D00"
        d="M6.3 14.7l6.6 4.8C14.7 15.1 19 12 24 12c3.1 0 5.8 1.2 7.9 3.1l5.7-5.7C34 6.1 29.3 4 24 4 16.3 4 9.7 8.3 6.3 14.7z"
      />
      <path
        fill="#4CAF50"
        d="M24 44c5.2 0 9.9-2 13.4-5.2l-6.2-5.2C29.2 35.1 26.7 36 24 36c-5.2 0-9.6-3.3-11.3-7.9l-6.5 5C9.5 39.6 16.2 44 24 44z"
      />
      <path
        fill="#1976D2"
        d="M43.6 20.5H42V20H24v8h11.3c-.8 2.2-2.2 4.2-4.1 5.6l6.2 5.2C37 39.2 44 34 44 24c0-1.2-.1-2.3-.4-3.5z"
      />
    </svg>
  );
}

// Full-page redirect to Google; the result comes back on /oauth/google.
export function GoogleSignInButton({
  redirectTo,
  label,
  className,
}: {
  redirectTo?: string;
  label?: string;
  className?: string;
}) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);

  const start = async () => {
    setLoading(true);
    try {
      const url = await startGoogleLogin();
      rememberGoogleRedirect(redirectTo);
      window.location.assign(url);
    } catch (error: unknown) {
      toast.error(error instanceof Error ? t(error.message) : t("Google sign-in is not available."));
      setLoading(false);
    }
  };

  return (
    <Button
      type="button"
      variant="outline"
      className={cn("h-10 w-full justify-center gap-2.5 rounded-xl font-medium", className)}
      disabled={loading}
      onClick={start}
    >
      {loading ? <Loader2 className="animate-spin" aria-hidden="true" /> : <GoogleIcon className="size-4" />}
      {label ?? t("Continue with Google")}
    </Button>
  );
}

export function AuthDivider({ label }: { label: string }) {
  return (
    <div className="text-muted-foreground flex items-center gap-3 text-xs">
      <span className="bg-border h-px flex-1" />
      <span>{label}</span>
      <span className="bg-border h-px flex-1" />
    </div>
  );
}
