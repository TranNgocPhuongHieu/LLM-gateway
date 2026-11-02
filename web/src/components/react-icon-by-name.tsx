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
import { CreditCard, Landmark, WalletCards } from 'lucide-react'
import { useEffect, useState } from 'react'
import type { LucideIcon } from 'lucide-react'
import type { IconBaseProps } from 'react-icons'

function normalizeIconName(name: string | null | undefined): string | null {
  const trimmed = name?.trim()
  if (!trimmed || !/^[A-Z][A-Za-z0-9]*$/.test(trimmed)) return null
  return trimmed
}

const ICON_MAP: Record<string, LucideIcon> = {
  LuCreditCard: CreditCard,
  LuWalletCards: WalletCards,
  LuLandmark: Landmark,
}

type ReactIconByNameProps = IconBaseProps & {
  name?: string | null
}

type ResolvedIconState = {
  iconName: string
  Icon: LucideIcon | null
}

export function ReactIconByName({ name, ...props }: ReactIconByNameProps) {
  const iconName = normalizeIconName(name)
  const [resolvedIcon, setResolvedIcon] = useState<ResolvedIconState | null>(
    null
  )

  useEffect(() => {
    if (!iconName) return

    setResolvedIcon({ iconName, Icon: ICON_MAP[iconName] ?? CreditCard })

    return () => {
      // No asynchronous icon pack remains to cancel.
    }
  }, [iconName])

  if (!iconName || resolvedIcon?.iconName !== iconName || !resolvedIcon.Icon) {
    return null
  }

  const Icon = resolvedIcon.Icon

  return <Icon {...props} />
}
