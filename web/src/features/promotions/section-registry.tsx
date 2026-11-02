import type { TFunction } from 'i18next'

export function getPromotionSectionNavItems(t: TFunction) {
  return [{ title: t('Promotions'), url: '/system-settings/promotions' }]
}
