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
import { useTranslation } from 'react-i18next'

export function Features(_props: { className?: string }) {
  const { t } = useTranslation()
  const features = [
    ['Multi-protocol Compatible', 'Compatible API routes for common AI application workflows'],
    ['Transparent Billing', 'Pay-as-you-go with real-time usage monitoring'],
    ['Security & Access', 'Multi-user management with flexible permission allocation'],
    ['Developer Friendly', 'Track usage, costs and performance with real-time analytics'],
  ]
  return (
    <section className='px-5 py-12 sm:px-6'>
      <div className='mx-auto max-w-6xl'>
        <h2 className='text-2xl font-semibold'>{t('Built for developers,')}</h2>
        <p className='text-muted-foreground mt-2 text-sm'>{t('designed for scale')}</p>
        <div className='mt-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
          {features.map(([title, description]) => (
            <article key={title} className='rounded-2xl border border-[#26282d] bg-[#17181b] p-5'>
              <h3 className='text-sm font-semibold'>{t(title)}</h3>
              <p className='text-muted-foreground mt-3 text-sm leading-relaxed'>{t(description)}</p>
            </article>
          ))}
        </div>
      </div>
    </section>
  )
}
