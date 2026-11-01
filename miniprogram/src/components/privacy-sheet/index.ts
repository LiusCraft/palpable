// 隐私弹窗：首次打开（visible）或平台要求隐私授权时弹出。
// "同意"按钮带 open-type="agreePrivacyAuthorization"，一次点击同时完成我们的同意记录与微信的隐私授权。
import { markAgreed, onNeedAuthorization } from '../../services/privacy'
import { copy } from '../../utils/copy'

type Resolver = (r: { event: 'agree' | 'disagree'; buttonId?: string }) => void

Component({
  properties: {
    visible: { type: Boolean, value: false },
  },
  data: {
    copy: copy.privacy,
    platformAsk: false,
  },
  lifetimes: {
    attached() {
      const self = this as unknown as { off?: () => void; resolver?: Resolver }
      self.off = onNeedAuthorization((resolve) => {
        self.resolver = resolve
        this.setData({ platformAsk: true })
      })
    },
    detached() {
      const self = this as unknown as { off?: () => void }
      self.off?.()
    },
  },
  methods: {
    openDoc(e: WechatMiniprogram.TouchEvent) {
      wx.navigateTo({ url: `/pages/legal/index?doc=${e.currentTarget.dataset.doc}` })
    },
    onAgree() {
      markAgreed()
      const self = this as unknown as { resolver?: Resolver }
      self.resolver?.({ event: 'agree', buttonId: 'privacy-agree' })
      self.resolver = undefined
      this.setData({ platformAsk: false })
      this.triggerEvent('agree')
    },
    onDisagree() {
      const self = this as unknown as { resolver?: Resolver }
      self.resolver?.({ event: 'disagree' })
      self.resolver = undefined
      this.setData({ platformAsk: false })
      this.triggerEvent('disagree')
    },
  },
})
