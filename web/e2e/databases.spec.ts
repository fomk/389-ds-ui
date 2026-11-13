import { expect, test } from '@playwright/test'
import { signIn, stamp } from './helpers'

test('create a suffix with its root entry, then delete it', async ({ page }) => {
  const name = `e2e${stamp()}`
  const suffix = `dc=${name},dc=test`
  await signIn(page, '/databases')

  await page.getByRole('button', { name: 'New suffix' }).click()
  await page.getByRole('textbox', { name: 'Suffix', exact: true }).fill(suffix)
  await page.getByRole('textbox', { name: 'Backend name' }).fill(name)
  await page.getByRole('dialog').getByRole('button', { name: 'Create', exact: true }).click()
  await expect(page.getByText(`Suffix ${suffix} created`)).toBeVisible()
  await expect(page.getByRole('link', { name: 'Open in LDAP browser' })).toBeVisible()

  await page.getByRole('button', { name: 'Delete suffix…' }).click()
  await page.getByLabel(/To confirm, type/).fill(name)
  await page.getByRole('button', { name: 'Delete suffix and data' }).click()
  await expect(page.getByText(`Suffix ${suffix} deleted`)).toBeVisible()
})
