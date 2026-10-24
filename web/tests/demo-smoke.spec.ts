import { expect, test } from '@playwright/test'

test('演示客户端可以进入第一人称房间', async ({ page }) => {
  await page.goto('/#/play', { waitUntil: 'domcontentloaded', timeout: 15_000 })
  await page.getByRole('button', { name: '就选它，走进房间' }).click()

  await expect(page.locator('canvas')).toBeVisible()
  await expect(page.getByText('点一下画面，用鼠标看四周（Esc 放开鼠标）')).toBeVisible()
})
