import { expect, test } from '@playwright/test'
import { signIn } from './helpers'

test('health check runs and links to where findings are fixed', async ({ page }) => {
  await signIn(page, '/health')
  await expect(page.getByText(/\d+ checks ·/)).toBeVisible()
  await expect(page.getByText(/could not run/)).toHaveCount(0)
  // The test servers allow anonymous access (the 389-ds default).
  const finding = page.locator('.finding').filter({ hasText: 'Anonymous access is allowed' })
  await expect(finding).toBeVisible()
  await finding.getByRole('link', { name: 'Open' }).click()
  await expect(page).toHaveURL(/\/settings\?page=security-binds/)
})
