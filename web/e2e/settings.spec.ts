import { expect, test } from '@playwright/test'
import { signIn } from './helpers'

test('change a server setting and restore it', async ({ page }) => {
  await signIn(page, '/settings?page=server-limits')
  const field = page.getByLabel('Size limit', { exact: true })
  await expect(field).toBeVisible()
  const before = await field.inputValue()
  const changed = before === '1999' ? '1998' : '1999'

  await field.fill(changed)
  await field.press('Tab') // the number input commits on blur
  await expect(page.getByText(/1 unsaved change/)).toBeVisible()
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.getByText('Settings saved.')).toBeVisible()

  await page.reload()
  await expect(page.getByLabel('Size limit', { exact: true })).toHaveValue(changed)

  await page.getByLabel('Size limit', { exact: true }).fill(before)
  await page.getByLabel('Size limit', { exact: true }).press('Tab')
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.getByText('Settings saved.')).toBeVisible()
})
