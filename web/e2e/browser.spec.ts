import { expect, test } from '@playwright/test'
import { signIn, stamp } from './helpers'

const people = 'ou=People,dc=example,dc=com'

test('create, edit and delete an entry in the LDAP browser', async ({ page }) => {
  const name = `e2e-${stamp()}`
  const dn = `ou=${name},${people}`
  await signIn(page, '/browser?dn=' + encodeURIComponent(people))

  // Create an organizational unit under People.
  await page.getByRole('button', { name: 'New entry' }).click()
  await page.getByText('Organizational unit', { exact: true }).click()
  await page.getByRole('textbox', { name: 'ou', exact: true }).fill(name)
  await page.getByRole('textbox', { name: 'description', exact: true }).fill('created by e2e')
  await expect(page.locator('code').filter({ hasText: dn })).toBeVisible()
  await page.getByRole('button', { name: 'Create', exact: true }).click()
  await expect(page.getByText(`Created ${dn}`)).toBeVisible()
  await expect(page.getByRole('cell', { name: 'created by e2e' })).toBeVisible()

  // Change the description.
  await page.getByRole('button', { name: 'Edit', exact: true }).click()
  await page.getByRole('textbox', { name: 'description', exact: true }).fill('changed by e2e')
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.getByText('Changes saved')).toBeVisible()
  await expect(page.getByRole('cell', { name: 'changed by e2e' })).toBeVisible()

  // Delete it.
  await page.getByRole('button', { name: 'More actions' }).click()
  await page.getByText('Delete', { exact: true }).click()
  await page.getByRole('dialog').getByRole('button', { name: 'Delete', exact: true }).click()
  await expect(page.getByText('Deleted 1 entry')).toBeVisible()
})
