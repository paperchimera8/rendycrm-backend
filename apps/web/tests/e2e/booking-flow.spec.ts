import { expect, test } from '@playwright/test'

function localDateTime(daysFromToday: number, hour: number, minute: number) {
  const date = new Date()
  date.setDate(date.getDate() + daysFromToday)
  date.setHours(hour, minute, 0, 0)

  const year = date.getFullYear()
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  const hours = `${date.getHours()}`.padStart(2, '0')
  const minutes = `${date.getMinutes()}`.padStart(2, '0')

  return `${year}-${month}-${day}T${hours}:${minutes}`
}

test('confirm, reschedule and cancel a booking from dialogs', async ({ page }) => {
  const initialStart = localDateTime(1, 12, 0)
  const initialEnd = localDateTime(1, 13, 0)
  const movedStart = localDateTime(1, 14, 0)
  const movedEnd = localDateTime(1, 15, 0)

  await page.goto('/login')
  await page.getByLabel('Email').fill('operator@rendycrm.local')
  await page.getByLabel('Пароль').fill('password')
  await page.getByTestId('login-submit').click()
  await expect(page).toHaveURL(/\/$/)

  await page.getByRole('link', { name: 'Диалоги' }).click()
  await page.getByTestId('conversation-cnv_3').click()

  await page.getByTestId('booking-start').fill(initialStart)
  await page.getByTestId('booking-end').fill(initialEnd)
  await page.getByTestId('booking-amount').fill('4500')
  await page.getByTestId('booking-submit').click()
  await expect(page.getByText('Подтвержденная запись')).toBeVisible()
  await expect(page.getByTestId('booking-cancel')).toBeVisible()

  await page.getByRole('link', { name: 'Слоты' }).click()
  await expect(page.getByText('Elena Sidorova')).toBeVisible()
  await expect(page.locator('input[type="time"][value="12:00"]').first()).toBeVisible()

  await page.getByRole('link', { name: 'Диалоги' }).click()
  await page.getByTestId('conversation-cnv_3').click()
  await page.getByTestId('booking-start').fill(movedStart)
  await page.getByTestId('booking-end').fill(movedEnd)
  await page.getByTestId('booking-amount').fill('4700')
  await page.getByTestId('booking-submit').click()
  await expect(page.getByTestId('booking-submit')).toHaveText('Перенести')

  await page.getByRole('link', { name: 'Слоты' }).click()
  await expect(page.locator('input[type="time"][value="14:00"]').first()).toBeVisible()

  await page.getByRole('link', { name: 'Диалоги' }).click()
  await page.getByTestId('conversation-cnv_3').click()
  await page.locator('[data-testid="booking-cancel"]').evaluate((element) => {
    ;(element as HTMLButtonElement).click()
  })
  await expect(page.getByTestId('booking-submit')).toHaveText('Подтвердить запись')
})
