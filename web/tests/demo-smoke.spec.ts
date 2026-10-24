import { expect, test } from '@playwright/test'

test('演示客户端可以进入第一人称房间', async ({ page }) => {
  await page.goto('/#/play', { waitUntil: 'domcontentloaded', timeout: 15_000 })
  await page.getByRole('button', { name: '就选它，走进房间' }).click()

  await expect(page.locator('canvas')).toBeVisible()
  // Pointer lock may be granted by Chromium immediately after the entry click,
  // so the optional mouse hint is not a stable assertion target.
  await expect(page.getByRole('link', { name: '👩 家长端' })).toBeVisible()
})
