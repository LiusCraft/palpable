// 小伙伴头像：皮套决定插画与底色；可选在线状态点。
import { skinLook } from '../../utils/skins'

Component({
  properties: {
    skin: { type: String, value: '' },
    size: { type: Number, value: 120 },
    status: { type: String, value: '' }, // '' 不显示；online / offline
  },
  data: { image: '', tint: '' },
  observers: {
    skin(skin: string) {
      const look = skinLook(skin)
      this.setData({ image: look.image, tint: look.tint })
    },
  },
  lifetimes: {
    attached() {
      const look = skinLook(this.data.skin)
      this.setData({ image: look.image, tint: look.tint })
    },
  },
})
