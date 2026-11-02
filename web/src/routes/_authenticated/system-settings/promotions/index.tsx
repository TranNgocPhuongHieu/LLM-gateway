import { createFileRoute } from '@tanstack/react-router'
import { PromotionsSettings } from '@/features/promotions'

export const Route = createFileRoute('/_authenticated/system-settings/promotions/')({
  component: PromotionsSettings,
})
