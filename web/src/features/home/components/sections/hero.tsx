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
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { openAuthDialog } from '@/stores/auth-dialog-store'
import { HeroTerminalDemo } from '../hero-terminal-demo'

export function Hero({ isAuthenticated = false }: { isAuthenticated?: boolean; className?: string }) {
  const { t } = useTranslation()
  return (
    <section className='px-5 pt-28 pb-14 sm:px-6 lg:pt-36 lg:pb-20'>
      <div className='mx-auto grid max-w-6xl items-center gap-10 lg:grid-cols-2 lg:gap-14'>
        <div>
          <span className='inline-flex rounded-full bg-primary/10 px-3 py-1 text-xs font-medium text-blue-500 dark:text-blue-400'>{t('AI API platform at an affordable price')}</span>
          <h1 className='mt-4 text-4xl leading-tight font-bold tracking-tight sm:text-5xl'>
            {t('Affordable AI solutions')}
          </h1>
          <p className='text-muted-foreground mt-5 max-w-xl text-base leading-relaxed'>{t('Access leading AI models at optimized cost, using standard API protocols with stable performance.')}</p>
          <div className='mt-6 flex flex-wrap gap-2'>
            {isAuthenticated ? (
            <Button className='h-11 px-5' render={<Link to='/dashboard' />}>{t('Access Dashboard')}</Button>
            ) : (
              <Button className='h-11 px-5' onClick={() => openAuthDialog('sign-in')}>{t('Access Dashboard')}</Button>
            )}
            <Button className='h-11 px-5' variant='outline' render={<a href='#pricing-preview' />}>{t('View pricing')}</Button>
          </div>
          <p className='text-muted-foreground mt-7 text-sm'>{t('Works with')}</p>
          <div className='mt-4 flex flex-wrap gap-2'>
            {['Claude Code', 'Codex', 'Kilo Code', 'OpenClaw', 'Roo Code', 'Many more'].map((name) => <span key={name} className='rounded-full border px-4 py-2 text-sm hover:bg-muted'>{t(name)}</span>)}
          </div>
        </div>
        <HeroTerminalDemo />
      </div>
    </section>
  )
}
