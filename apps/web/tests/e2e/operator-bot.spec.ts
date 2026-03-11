import { expect, test } from '@playwright/test'

test('operator bot link code can be created from settings', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('Email').fill('operator@rendycrm.local')
  await page.getByLabel('Пароль').fill('password')
  await page.getByTestId('login-submit').click()
  await expect(page).toHaveURL(/\/$/)

  await page.getByRole('link', { name: 'Настройки' }).click()
  await expect(page.getByText('Telegram operator bot')).toBeVisible()

  await page.getByRole('button', { name: 'Создать ссылку' }).click()
  await expect(page.getByText('Активная ссылка:')).toBeVisible()
  await expect(page.getByText(/https:\/\/t\.me\//)).toBeVisible()
})
