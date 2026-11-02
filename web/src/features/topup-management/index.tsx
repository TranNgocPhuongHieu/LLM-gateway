import { RefreshCw } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { SectionPageLayout } from "@/components/layout";
import { TopupManagementTable } from "./topup-management-table";

export function TopupManagementPage() {
  const { t } = useTranslation();
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <span className="block text-2xl font-bold">{t("Top-up history")}</span>
        <span className="mt-1 block text-sm font-normal text-muted-foreground">
          {t("Track and reconcile top-up transactions.")}
        </span>
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          type="submit"
          variant="outline"
          size="default"
          className="min-h-11"
          form="topup-management-refresh"
        >
          <RefreshCw className="mr-2 size-4" />
          {t("Refresh")}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <TopupManagementTable />
      </SectionPageLayout.Content>
    </SectionPageLayout>
  );
}
