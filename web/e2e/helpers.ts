import { readFileSync } from 'node:fs'
import { expect, type Page } from '@playwright/test'

/** Directory Manager password: E2E_PASSWORD or the development secret. */
export function dmPassword(): string {
  if (process.env.E2E_PASSWORD) return process.env.E2E_PASSWORD
  return readFileSync(new URL('../../secrets/dm_password', import.meta.url), 'utf8').trim()
}

/** Signs in as Directory Manager in English and lands on path. */
export async function signIn(page: Page, path = '/') {
  await page.addInitScript(() => localStorage.setItem('dsui.locale', 'en'))
  await page.goto(path)
  if (page.url().includes('/login')) {
    await page.getByLabel('User').fill('Directory Manager')
    await page.getByLabel('Password').fill(dmPassword())
    await page.getByRole('button', { name: 'Sign in' }).click()
  }
  // The user menu shows the name and the role.
  await expect(page.getByRole('button', { name: /^Directory Manager (administrator|operator)$/ })).toBeVisible()
}

/** Unique suffix for names the tests create. */
export const stamp = () => Date.now().toString(36)
