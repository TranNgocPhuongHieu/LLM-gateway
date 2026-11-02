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
import { Link } from '@tanstack/react-router'

export function HowItWorks() {
  const { t } = useTranslation()
  const steps = [
    ['Configure', 'Tạo tài khoản, kiểm tra số dư rồi tạo và bật API key.'],
    ['Connect', 'Kết nối Codex, Claude Code, Kilo Code hoặc Roo Code theo hướng dẫn.'],
    ['Monitor', 'Theo dõi mức sử dụng, chi phí và hiệu suất theo thời gian thực.'],
  ]
  return (
    <section className='px-5 py-12 pb-20 sm:px-6'>
      <div className='mx-auto max-w-6xl'>
        <h2 className='text-2xl font-semibold'>{t('Three steps to get started')}</h2>
        <div className='mt-6 grid gap-4 md:grid-cols-3'>
          {steps.map(([title, description], index) => (
            <article key={title} className='rounded-2xl border border-[#26282d] bg-[#17181b] p-5'>
              <span className='inline-flex size-8 items-center justify-center rounded-full bg-primary/15 text-sm font-semibold text-blue-500 dark:text-blue-400'>{index + 1}</span>
              <h3 className='mt-3 text-base font-semibold'>{t(title)}</h3>
              <p className='text-muted-foreground mt-2 text-sm leading-relaxed'>{t(description)}</p>
            </article>
          ))}
        </div>
        <Link to='/docs/$page' params={{ page: 'quickstart' }} className='mt-5 inline-block text-sm font-medium text-primary'>{t('Docs')} →</Link>
      </div>
    </section>
  )
}
