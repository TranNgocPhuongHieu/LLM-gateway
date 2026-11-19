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
import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { RichContent } from '@/components/rich-content'
import { api } from '@/lib/api'
import { isHttpUrl, isLikelyHtml } from '@/lib/content-format'
import { requireServerSuccess } from '@/lib/server-error-message'

interface UserAgreementResponse {
  success: boolean
  message?: string
  data?: string
}

// Public endpoint: the agreement text (Markdown, HTML or an external URL) is
// configured by the super admin. Drop the global `no-store` header so the
// browser can revalidate with the server's ETag.
async function fetchUserAgreement(): Promise<string> {
  const res = await api.get<UserAgreementResponse>('/api/user-agreement', {
    headers: { 'Cache-Control': null },
  })
  return requireServerSuccess(res.data).data ?? ''
}

export const Route = createFileRoute('/user-agreement')({
  component: UserAgreementPage,
})

function UserAgreementPage() {
  const { t } = useTranslation()
  const agreementQuery = useQuery({
    queryKey: ['user-agreement'],
    queryFn: fetchUserAgreement,
  })

  const content = agreementQuery.data?.trim() ?? ''

  let body
  if (agreementQuery.isError) {
    body = (
      <ErrorState
        title={t('Could not load the user agreement')}
        onRetry={() => agreementQuery.refetch()}
      />
    )
  } else if (agreementQuery.isPending) {
    body = <p className='text-muted-foreground'>{t('Loading...')}</p>
  } else if (!content) {
    body = (
      <p className='text-muted-foreground'>
        {t('No user agreement has been published yet.')}
      </p>
    )
  } else if (isHttpUrl(content)) {
    body = (
      <a
        className='text-primary underline underline-offset-4'
        href={content}
        target='_blank'
        rel='noopener noreferrer'
      >
        {t('Open the user agreement')}
      </a>
    )
  } else if (isLikelyHtml(content)) {
    body = <RichContent mode='html' content={content} />
  } else {
    body = <RichContent mode='markdown' content={content} />
  }

  return (
    <main className='mx-auto max-w-2xl space-y-5 px-6 py-12'>
      <h1 className='text-2xl font-bold'>{t('User Agreement')}</h1>
      {body}
    </main>
  )
}
