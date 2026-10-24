import { expect, test } from '@playwright/test'

test('演示主线可以从开箱进入公园频道', async ({ page }) => {
  await page.goto('/')

  await page.getByRole('button', { name: '打开盒子' }).click()
  await page.getByRole('button', { name: '把它抱出来' }).click()
  await page.getByRole('button', { name: '放到桌面上' }).click()

  await expect(page.getByRole('heading', { name: '命名认主' })).toBeVisible()
  await expect(page.getByText('已上线')).toBeVisible({ timeout: 15_000 })

  await page.getByRole('button', { name: /去公园玩/ }).click()
  const map = page.getByTestId('park-map')
  await expect(map).toBeVisible()

  const box = await map.boundingBox()
  expect(box).not.toBeNull()
  if (!box) return

  // 初始位置由 park/startPoint 放在地图左侧；拖到公园中心触发服务端区域判定。
  const scaleX = box.width / 860
  const scaleY = box.height / 660
  await page.mouse.move(box.x + 44 * scaleX, box.y + 200 * scaleY)
  await page.mouse.down()
  await page.mouse.move(box.x + 430 * scaleX, box.y + 330 * scaleY, { steps: 5 })
  await page.mouse.up()

  await expect(page.getByText('要加入这个区域频道吗？')).toBeVisible({ timeout: 10_000 })
  await page.getByRole('button', { name: '加入', exact: true }).click()
  await expect(page.getByRole('heading', { name: '公园区域频道' })).toBeVisible({ timeout: 10_000 })
})
