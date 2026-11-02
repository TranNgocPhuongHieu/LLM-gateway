import { LanguageSwitcher } from '@/components/language-switcher'
import { NotificationPopover } from '@/components/notification-popover'
import { PromotionHeaderButton } from '@/components/promotion-header-button'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { Search } from '@/components/search'
import { useNotifications } from '@/hooks/use-notifications'
import { useTopNavLinks } from '@/hooks/use-top-nav-links'
import { defaultTopNavLinks } from '../config/top-nav.config'
import type { TopNavLink } from '../types'
import { Header } from './header'
import { SystemBrand } from './system-brand'
import { TopNav } from './top-nav'

type AppHeaderProps = { navLinks?: TopNavLink[]; showTopNav?: boolean; leftContent?: React.ReactNode; showSearch?: boolean; rightContent?: React.ReactNode; showNotifications?: boolean; showProfileDropdown?: boolean }

export function AppHeader({ navLinks = defaultTopNavLinks, showTopNav = true, leftContent, showSearch = true, rightContent, showNotifications = false, showProfileDropdown = true }: AppHeaderProps) {
  const dynamicLinks = useTopNavLinks()
  const links = dynamicLinks.length > 0 ? dynamicLinks : navLinks
  const notifications = useNotifications()
  return <Header><div className='@container/system-brand flex min-w-0 flex-1 items-center gap-1'><SystemBrand variant='inline' /></div>{leftContent ? <div className='ms-2 flex items-center'>{leftContent}</div> : null}{rightContent ?? <div className='ms-auto flex shrink-0 items-center gap-2 sm:gap-3'>{showTopNav && <div className='me-1 hidden lg:block'><TopNav links={links} /></div>}{showSearch && <Search className='h-9 w-9 flex-none rounded-xl border bg-card px-3.5 text-xs text-muted-foreground sm:w-64 sm:justify-start' />}<PromotionHeaderButton />{showNotifications && <NotificationPopover open={notifications.popoverOpen} onOpenChange={notifications.setPopoverOpen} unreadCount={notifications.unreadCount} activeTab={notifications.activeTab} onTabChange={notifications.setActiveTab} notice={notifications.notice} announcements={notifications.announcements} loading={notifications.loading} />}<LanguageSwitcher />{showProfileDropdown && <ProfileDropdown />}</div>}</Header>
}
