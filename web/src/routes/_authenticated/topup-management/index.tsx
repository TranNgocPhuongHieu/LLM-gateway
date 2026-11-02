import { createFileRoute, redirect } from "@tanstack/react-router";

import { TopupManagementPage } from "@/features/topup-management";
import { ROLE } from "@/lib/roles";
import { useAuthStore } from "@/stores/auth-store";

export const Route = createFileRoute("/_authenticated/topup-management/")({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState();
    if (!auth.user || auth.user.role < ROLE.SUPER_ADMIN) {
      throw redirect({ to: "/403" });
    }
  },
  component: TopupManagementPage,
});
