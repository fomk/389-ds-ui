import { expect, test } from '@playwright/test'
import { dmPassword, signIn } from './helpers'

test('wrong password is refused, then sign in and out', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('dsui.locale', 'en'))
  await page.goto('/monitor')
  await expect(page).toHaveURL(/\/login\?next=/)
  await page.getByLabel('User').fill('Directory Manager')
  await page.getByLabel('Password').fill('not-the-password')
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('alert')).toContainText(/wrong|invalid|incorrect/i)

  await page.getByLabel('Password').fill(dmPassword())
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page).toHaveURL(/\/monitor$/)

  await page.getByRole('button', { name: /Directory Manager/ }).click()
  await page.getByText('Sign out').click()
  await expect(page).toHaveURL(/\/login/)
})

test('signed-in pages render without errors', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', (e) => errors.push(e.message))
  await signIn(page)
  for (const path of ['/', '/browser', '/monitor', '/health', '/databases', '/logs', '/settings', '/plugins', '/schema', '/policies', '/roles', '/replication', '/security']) {
    await test.step(path, async () => {
      await page.goto(path)
      await page.waitForLoadState('networkidle')
      await expect(page.locator('.content')).not.toBeEmpty()
      // Untranslated keys show up as dotted identifiers.
      await expect(page.locator('.content')).not.toContainText(/\b(pages|health|tls|db|nav|plugins|login)\.[a-z][\w-]*\.[\w.-]+/)
    })
  }
  expect(errors).toEqual([])
})
