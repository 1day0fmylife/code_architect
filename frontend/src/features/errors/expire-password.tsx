import { useState } from 'react'
import { AlertTriangle, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { ChangePasswordForm } from '@/features/settings/change-password'

export function ExpiredPassword() {
  const { t } = useTranslation("password")
  const { logout } = useAuthStore()
  const [showChangePassword, setShowChangePassword] = useState(false)

  const handleLogout = async () => {
    await logout()
  }

  if (showChangePassword) {
    return (
      <div className="min-h-screen bg-background flex items-center justify-center p-4">
        <div className="w-full max-w-2xl">
          <div className="flex justify-between items-center mb-4">
            <h1 className="text-2xl font-bold">{t('changePassword')}</h1>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => setShowChangePassword(false)}
            >
              <X className="w-5 h-5" />
            </Button>
          </div>
          <ChangePasswordForm />
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-background flex items-center justify-center p-4">
      <Card className="w-full max-w-md shadow-lg">
        <CardHeader className="text-destructive-foreground rounded-t-lg text-center py-2">
          <div className="flex justify-center mb-4">
            <div className="relative">
              <AlertTriangle className="w-16 h-16 text-destructive" />
            </div>
          </div>
          <CardTitle className="text-2xl">{t('passwordExpired')}</CardTitle>
          <CardDescription className="text-destructive-foreground/80 mt-2">
            {t('passwordExpiredReason')}
          </CardDescription>
        </CardHeader>

        <CardContent className="space-y-6">
          <Alert variant="destructive">
            <AlertDescription>
              {t('mustChangePassword')}
            </AlertDescription>
          </Alert>

          <div className="space-y-3">
            <h3 className="font-semibold text-foreground">{t('securityGuidelines')}:</h3>
            <ul className="space-y-2 text-sm text-muted-foreground">
              <li className="flex gap-2">
                <span className="text-destructive font-bold">•</span>
                <span>{t('expiresAfterPeriod')}</span>
              </li>
              <li className="flex gap-2">
                <span className="text-destructive font-bold">•</span>
                <span>{t('useStrongPassword')}</span>
              </li>
              <li className="flex gap-2">
                <span className="text-destructive font-bold">•</span>
                <span>{t('dontReusePrevious')}</span>
              </li>
            </ul>
          </div>

          <div className="space-y-3 pt-4">
            <Button
              onClick={() => setShowChangePassword(true)}
              className="w-full"
              size="lg"
            >
              {t('changePasswordNow')}
            </Button>

            <Button
              onClick={handleLogout}
              variant="outline"
              className="w-full"
              size="lg"
            >
              {t('common:logout')}
            </Button>
          </div>

          <p className="text-xs text-muted-foreground text-center pt-2">
            {t('contactAdministrator')}
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
