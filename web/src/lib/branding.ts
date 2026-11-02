export function getLocalizedSystemName(rawName?: string, _lang?: string): string {
  if (
    !rawName ||
    rawName === 'New API' ||
    rawName === 'LLM' ||
    rawName === 'AI giá rẻ' ||
    rawName === 'AI cheap' ||
    rawName === 'API mới'
  ) {
    return 'LLM'
  }
  return rawName
}

