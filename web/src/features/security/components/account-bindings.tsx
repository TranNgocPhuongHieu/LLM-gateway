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
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { SecureVerificationDialog } from "@/features/auth/secure-verification";
import { GoogleIcon } from "@/features/auth/components/google-sign-in-button";
import { Button } from "@/components/ui/button";
import { startGoogleBinding } from "@/features/profile/api";
import type { UserProfile } from "@/features/profile/types";
import { useStatus } from "@/hooks/use-status";
import { useAccountSecurity } from "../hooks/use-account-security";
import { EmailBindDialog } from "./dialogs/email-bind-dialog";

export function AccountBindings({
  profile,
  onUpdate,
}: {
  profile: UserProfile | null;
  onUpdate: () => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const { status } = useStatus();
  const security = useAccountSecurity();
  const googleEnabled = status?.google_oauth_enabled === true;

  const linkGoogle = async () => {
    const result = await security.run(async (signal) => {
      const proof = await security.verify(
        { scope: "account.binding.bind", context: { provider: "google" } },
        signal,
      );
      return startGoogleBinding(proof, signal);
    });
    if (result?.url) window.location.assign(result.url);
  };

  return (
    <div className="space-y-4">
      <div className="space-y-3">
        <p className="text-sm">{profile?.email || t("Not bound")}</p>
        <Button variant="outline" onClick={() => setOpen(true)}>
          {t("Bind Email")}
        </Button>
      </div>
      {(googleEnabled || profile?.google_linked) && (
        <div className="flex flex-wrap items-center justify-between gap-3 border-t pt-4">
          <div className="flex items-center gap-2 text-sm">
            <GoogleIcon className="size-4" />
            <span className="font-medium">Google</span>
            <span className="text-muted-foreground">
              {profile?.google_linked ? t("Linked") : t("Not linked")}
            </span>
          </div>
          {!profile?.google_linked && (
            <Button variant="outline" disabled={security.pending} onClick={linkGoogle}>
              {t("Link Google")}
            </Button>
          )}
        </div>
      )}
      <EmailBindDialog open={open} onOpenChange={setOpen} onSuccess={onUpdate} />
      {security.showVerification && (
        <SecureVerificationDialog {...security.verificationDialogProps} />
      )}
    </div>
  );
}
