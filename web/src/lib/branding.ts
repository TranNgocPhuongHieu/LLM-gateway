import i18next from 'i18next'

export function getLocalizedSystemName(rawName?: string, lang?: string): string {
  const currentLang = (lang || i18next.language || 'vi').toLowerCase()
  const isEn = currentLang.startsWith('en')

  if (
    !rawName ||
    rawName === 'New API' ||
    rawName === 'AI giá rẻ' ||
    rawName === 'AI cheap' ||
    rawName === 'API mới'
  ) {
    return isEn ? 'AI cheap' : 'AI giá rẻ'
  }
  return rawName
}

