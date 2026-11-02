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
import {
  Activity,
  Box,
  FileText,
  Gauge,
  Key,
  LayoutDashboard,
  Radio,
  ServerCog,
  Settings,
  ShieldCheck,
  Tag,
  User,
  Users,
  Wallet,
  Banknote,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import type { SidebarData } from "@/components/layout/types";
import { ROLE } from "@/lib/roles";

/**
 * Root navigation groups for the application sidebar.
 *
 * These are shown when the URL does not match any nested sidebar view
 * registered in `layout/lib/sidebar-view-registry.ts`.
 */
export function useSidebarData(): SidebarData {
  const { t } = useTranslation();

  return {
    navGroups: [
      {
        id: "general",
        title: t("General"),
        items: [
          {
            title: t("Overview"),
            url: "/dashboard/overview",
            icon: Activity,
          },
          {
            title: t("Dashboard"),
            url: "/dashboard/models",
            activeUrls: ["/dashboard/logs", "/dashboard/users"],
            icon: LayoutDashboard,
          },
          {
            title: t("API Keys"),
            url: "/keys",
            icon: Key,
          },
        ],
      },
      {
        id: "personal",
        title: t("Personal"),
        items: [
          {
            title: t("Wallet"),
            url: "/wallet",
            icon: Wallet,
          },
          {
            title: t("Profile"),
            url: "/profile",
            icon: User,
          },
          {
            title: t("Security & Access"),
            url: "/security",
            icon: ShieldCheck,
          },
        ],
      },
      {
        id: "admin",
        title: t("Admin"),
        items: [
          {
            title: t("Channels"),
            url: "/channels",
            icon: Radio,
            requiredRole: ROLE.SUPER_ADMIN,
          },
          {
            title: t("Quota Check"),
            url: "/quota-check",
            icon: Gauge,
            requiredRole: ROLE.SUPER_ADMIN,
          },
          {
            title: t("Price Management"),
            url: "/price-management",
            icon: Tag,
            requiredRole: ROLE.SUPER_ADMIN,
          },
          {
            title: t("Request Logs"),
            url: "/request-logs",
            icon: FileText,
            requiredRole: ROLE.SUPER_ADMIN,
          },
          {
            title: t("Top-up Management"),
            url: "/topup-management",
            icon: Banknote,
            requiredRole: ROLE.SUPER_ADMIN,
          },
          {
            title: t("Models"),
            url: "/models/metadata",
            icon: Box,
            requiredRole: ROLE.SUPER_ADMIN,
          },
          {
            title: t("Users"),
            url: "/users",
            icon: Users,
            requiredRole: ROLE.SUPER_ADMIN,
          },
          {
            title: t("System Info"),
            url: "/system-info",
            icon: ServerCog,
            requiredRole: ROLE.SUPER_ADMIN,
          },
          {
            title: t("System Settings"),
            url: "/system-settings/site",
            activeUrls: ["/system-settings"],
            icon: Settings,
            requiredRole: ROLE.SUPER_ADMIN,
          },
        ],
      },
    ],
  };
}
