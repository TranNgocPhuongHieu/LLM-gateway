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
import i18next from "i18next";
import { useState, useEffect, useCallback, useRef } from "react";
import { toast } from "sonner";

import { useDebounce } from "@/hooks/use-debounce";
import { handleServerError } from "@/lib/handle-server-error";
import { ROLE } from "@/lib/roles";
import { useAuthStore } from "@/stores/auth-store";

import {
  getUserBillingHistory,
  getAllBillingHistory,
  completeOrder,
  isApiSuccess,
} from "../api";
import type { TopupRecord, TopupSummary } from "../types";

// ============================================================================
// Billing History Hook
// ============================================================================

interface UseBillingHistoryOptions {
  /** Initial page number */
  initialPage?: number;
  /** Initial page size */
  initialPageSize?: number;
  refreshKey?: number;
  scope?: "self" | "all";
}

export function useBillingHistory(options: UseBillingHistoryOptions = {}) {
  const {
    initialPage = 1,
    initialPageSize = 10,
    refreshKey = 0,
    scope = "self",
  } = options;
  const userRole = useAuthStore((state) => state.auth.user?.role);
  const canManageAll = scope === "all" && (userRole ?? 0) >= ROLE.SUPER_ADMIN;

  const [records, setRecords] = useState<TopupRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(initialPage);
  const [pageSize, setPageSize] = useState(initialPageSize);
  const [keyword, setKeyword] = useState("");
  const debouncedKeyword = useDebounce(keyword);
  const requestIdRef = useRef(0);
  const [loading, setLoading] = useState(false);
  const [summary, setSummary] = useState<TopupSummary | null>(null);
  const [completing, setCompleting] = useState(false);

  /**
   * Fetch billing history
   */
  const fetchBillingHistory = useCallback(async () => {
    const requestId = ++requestIdRef.current;
    setLoading(true);
    try {
      const response = canManageAll
        ? await getAllBillingHistory(page, pageSize, debouncedKeyword)
        : await getUserBillingHistory(page, pageSize, debouncedKeyword);

      if (requestId !== requestIdRef.current) return;

      if (isApiSuccess(response) && response.data) {
        setRecords(response.data.items || []);
        setTotal(response.data.total || 0);
        if (scope === "all") setSummary(response.data.summary || null);
      } else {
        handleServerError(
          response,
          i18next.t("Failed to load billing history"),
        );
        setRecords([]);
        setTotal(0);
      }
    } catch (error) {
      if (requestId !== requestIdRef.current) return;
      handleServerError(error, i18next.t("Failed to load billing history"));
      setRecords([]);
      setTotal(0);
    } finally {
      if (requestId === requestIdRef.current) {
        setLoading(false);
      }
    }
  }, [canManageAll, debouncedKeyword, page, pageSize]);

  /**
   * Complete a pending order (admin only)
   */
  const handleCompleteOrder = useCallback(
    async (tradeNo: string) => {
      const record = records.find((item) => item.trade_no === tradeNo);
      if (!canManageAll || !record || record.payment_provider === "sepay")
        return false;
      setCompleting(true);
      try {
        const response = await completeOrder({ trade_no: tradeNo });
        if (!isApiSuccess(response)) {
          handleServerError(response, i18next.t("Failed to complete order"));
          return false;
        }
        toast.success(i18next.t("Order completed successfully"));
        await fetchBillingHistory();
        return true;
      } catch (error) {
        handleServerError(error, i18next.t("Failed to complete order"));
        return false;
      } finally {
        setCompleting(false);
      }
    },
    [canManageAll, records, fetchBillingHistory],
  );

  /**
   * Change page
   */
  const handlePageChange = useCallback((newPage: number) => {
    setPage(newPage);
  }, []);

  /**
   * Change page size
   */
  const handlePageSizeChange = useCallback((newPageSize: number) => {
    setPageSize(newPageSize);
    setPage(1); // Reset to first page when changing page size
  }, []);

  /**
   * Search by keyword
   */
  const handleSearch = useCallback((newKeyword: string) => {
    requestIdRef.current += 1;
    setKeyword(newKeyword);
    setPage(1); // Reset to first page when searching
  }, []);

  // Fetch data after the search draft has settled.
  useEffect(() => {
    if (keyword !== debouncedKeyword) return;

    fetchBillingHistory();
  }, [debouncedKeyword, fetchBillingHistory, keyword, refreshKey]);

  return {
    records,
    total,
    summary,
    page,
    pageSize,
    keyword,
    loading,
    completing,
    isAdmin: canManageAll,
    handlePageChange,
    handlePageSizeChange,
    handleSearch,
    handleCompleteOrder,
    refresh: fetchBillingHistory,
  };
}
