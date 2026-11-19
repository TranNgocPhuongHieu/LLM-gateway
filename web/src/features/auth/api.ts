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
import axios from "axios";

import { api, refreshAuthentication, type RefreshOutcome } from "@/lib/api";
import { useAuthStore } from "@/stores/auth-store";

import { clearPasswordEncryptionCache, encryptPassword } from "./lib/password-encryption";
import type {
  LoginPayload,
  LoginResponse,
  RegisterPayload,
  ApiResponse,
} from "./types";

// ============================================================================
// Authentication APIs
// ============================================================================

// ----------------------------------------------------------------------------
// Login & Logout
// ----------------------------------------------------------------------------

// User login with username and password
export async function login(payload: LoginPayload): Promise<LoginResponse> {
  const turnstile = payload.turnstile ?? "";
  try {
    let passwordFields:
      | { password: string }
      | { password_encrypted: string; encryption_key_id: string };
    if (payload.passwordEncryptionEnabled) {
      const encryptedPassword = await encryptPassword(payload.password);
      passwordFields = {
        password_encrypted: encryptedPassword.password_encrypted,
        encryption_key_id: encryptedPassword.encryption_key_id,
      };
    } else {
      passwordFields = { password: payload.password };
    }
    const res = await api.post<LoginResponse>(
      "/api/user/login",
      {
        username: payload.username,
        ...passwordFields,
      },
      {
        skipAuthRefresh: true,
        headers: { "X-Turnstile-Token": turnstile },
      },
    );
    if (payload.passwordEncryptionEnabled && !res.data?.success) {
      clearPasswordEncryptionCache();
    }
    return res.data;
  } catch (error: unknown) {
    if (payload.passwordEncryptionEnabled) {
      clearPasswordEncryptionCache();
    }
    throw error;
  }
}

// ----------------------------------------------------------------------------
// Google sign-in
// ----------------------------------------------------------------------------

const GOOGLE_REDIRECT_STORAGE_KEY = "google_oauth_redirect";

// Starts Google sign-in/sign-up and returns Google's consent URL. The server
// also sets an HttpOnly cookie that ties the round trip to this browser.
export async function startGoogleLogin(): Promise<string> {
  const res = await api.post<ApiResponse<{ url: string }>>(
    "/api/oauth/google/start",
    undefined,
    { skipAuthRefresh: true, skipBusinessError: true, skipErrorHandler: true },
  );
  if (!res.data?.success || !res.data.data?.url) {
    throw new Error(res.data?.message || "Google sign-in is not available.");
  }
  return res.data.data.url;
}

// Exchanges the one-time ticket from the callback for a session (or a
// login verification challenge, exactly like password sign-in).
export async function finishGoogleLogin(ticket: string): Promise<LoginResponse> {
  const res = await api.post<LoginResponse>(
    "/api/oauth/google/finish",
    { ticket },
    { skipAuthRefresh: true, skipBusinessError: true, skipErrorHandler: true },
  );
  return res.data;
}

export function rememberGoogleRedirect(redirectTo?: string) {
  try {
    if (redirectTo) sessionStorage.setItem(GOOGLE_REDIRECT_STORAGE_KEY, redirectTo);
    else sessionStorage.removeItem(GOOGLE_REDIRECT_STORAGE_KEY);
  } catch {
    // Storage may be unavailable (private mode); fall back to the dashboard.
  }
}

export function takeGoogleRedirect(): string | undefined {
  try {
    const value = sessionStorage.getItem(GOOGLE_REDIRECT_STORAGE_KEY) ?? undefined;
    sessionStorage.removeItem(GOOGLE_REDIRECT_STORAGE_KEY);
    return value;
  } catch {
    return undefined;
  }
}

interface LogoutRuntime {
  getExpectedSID: () => string | undefined;
  request: (expectedSID?: string) => Promise<ApiResponse>;
  refresh: () => Promise<RefreshOutcome>;
}

export async function executeLogout(
  runtime: LogoutRuntime,
  allowMismatchRecovery = true,
): Promise<ApiResponse> {
  try {
    return await runtime.request(runtime.getExpectedSID());
  } catch (error: unknown) {
    const code = axios.isAxiosError(error) ? error.response?.data?.code : undefined;
    if (
      allowMismatchRecovery &&
      axios.isAxiosError(error) &&
      error.response?.status === 409 &&
      code === "AUTH_SESSION_MISMATCH"
    ) {
      const outcome = await runtime.refresh();
      if (outcome.kind === "authenticated") {
        return executeLogout(runtime, false);
      }
      if (outcome.kind === "anonymous") {
        return { success: true, message: "" };
      }
    }
    throw error;
  }
}

// User logout
export async function logout(): Promise<ApiResponse> {
  return executeLogout({
    getExpectedSID: () => useAuthStore.getState().auth.session?.sid,
    request: async (sid) => {
      const res = await api.post("/api/user/auth/logout", undefined, {
        headers: sid ? { "X-Auth-Session": sid } : undefined,
        skipAuthRefresh: true,
        skipErrorHandler: true,
      });
      return res.data;
    },
    refresh: refreshAuthentication,
  });
}

// ----------------------------------------------------------------------------
// Password Management
// ----------------------------------------------------------------------------

// Send password reset email
export async function sendPasswordResetEmail(
  email: string,
  turnstile?: string,
): Promise<ApiResponse> {
  const res = await api.get("/api/reset_password", {
    params: { email },
    headers: { "X-Turnstile-Token": turnstile ?? "" },
  });
  return res.data;
}

// ----------------------------------------------------------------------------
// Registration
// ----------------------------------------------------------------------------

// User registration
export async function register(payload: RegisterPayload): Promise<ApiResponse> {
  const res = await api.post(`/api/user/register`, payload, {
    headers: { "X-Turnstile-Token": payload.turnstile ?? "" },
  });
  return res.data;
}

// Send email verification code
export async function sendEmailVerification(
  email: string,
  turnstile?: string,
): Promise<ApiResponse> {
  const res = await api.get("/api/verification", {
    params: { email },
    headers: { "X-Turnstile-Token": turnstile ?? "" },
  });
  return res.data;
}
