import { createFileRoute } from '@tanstack/react-router'

import { PublicQuotaPage } from '@/features/public-quota'

export const Route = createFileRoute('/quota/')({
  component: PublicQuotaPage,
})
