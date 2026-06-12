import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { useAuthStore } from '@/stores/auth-store'

export function NoAccessPage() {
  const navigate = useNavigate()
  const { t } = useTranslation('common')
  const { logout } = useAuthStore()

  return (
    <div className='h-svh'>
      <div className='m-auto flex h-full w-full max-w-2xl flex-col items-center justify-center gap-3 px-6 text-center'>
        <h1 className='text-4xl font-bold tracking-tight'>
          {t('no_access.title', { defaultValue: 'No Available Sections' })}
        </h1>
        <p className='text-muted-foreground max-w-xl'>
          {t('no_access.description', {
            defaultValue:
              'Your account is active, but no application sections are assigned yet. Contact an administrator to grant menu access or a role with available pages.',
          })}
        </p>
        <div className='mt-6 flex flex-wrap justify-center gap-4'>
          <Button onClick={() => navigate({ to: '/settings/change-password' })}>
            {t('no_access.change_password', {
              defaultValue: 'Change Password',
            })}
          </Button>
          <Button variant='outline' onClick={() => navigate({ to: '/settings' })}>
            {t('no_access.open_settings', { defaultValue: 'Open Settings' })}
          </Button>
          <Button variant='destructive' onClick={() => void logout()}>
            {t('logout')}
          </Button>
        </div>
      </div>
    </div>
  )
}
