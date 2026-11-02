import { create } from 'zustand'

export type AuthDialogMode = 'sign-in' | 'sign-up' | 'verification'

type AuthDialogState = {
  mode: AuthDialogMode | null
  redirectTo?: string
  open: (mode: AuthDialogMode, redirectTo?: string) => void
  setMode: (mode: AuthDialogMode) => void
  close: () => void
}

export const useAuthDialogState = create<AuthDialogState>((set) => ({
  mode: null,
  redirectTo: undefined,
  open: (mode, redirectTo) =>
    set((state) => ({
      mode,
      redirectTo: redirectTo ?? (state.mode ? state.redirectTo : undefined),
    })),
  setMode: (mode) => set({ mode }),
  close: () => set({ mode: null, redirectTo: undefined }),
}))

export function openAuthDialog(
  mode: AuthDialogMode = 'sign-in',
  redirectTo?: string
) {
  useAuthDialogState.getState().open(mode, redirectTo)
}
