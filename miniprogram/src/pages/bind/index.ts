// 绑定：家长输入玩偶播报的 6 位绑定码，勾选同意后玩偶才开始提供服务（docs/trust.md §3）。
import { api } from '../../services/api'
import { copy, texts } from '../../utils/copy'
import { errorText, toast } from '../../utils/ui'

Page({
  data: {
    t: texts(copy.bind),
    code: '',
    complete: false,
    consent: false,
    voice: false,
    submitting: false,
    codeError: false,
  },

  onCode(e: WechatMiniprogram.CustomEvent<{ value: string; complete: boolean }>) {
    this.setData({ code: e.detail.value, complete: e.detail.complete, codeError: false })
  },

  onConsent(e: WechatMiniprogram.CustomEvent<{ checked: boolean }>) {
    this.setData({ consent: e.detail.checked })
  },

  onVoice(e: WechatMiniprogram.CustomEvent<{ value: boolean }>) {
    this.setData({ voice: e.detail.value })
  },

  openDoc(e: WechatMiniprogram.TouchEvent) {
    wx.navigateTo({ url: `/pages/legal/index?doc=${e.currentTarget.dataset.doc}` })
  },

  async submit() {
    const { code, complete, consent, voice, submitting } = this.data
    if (submitting) return
    if (!complete) return toast(this, copy.bind.needCode, 'warning')
    if (!consent) return toast(this, copy.bind.needConsent, 'warning')
    this.setData({ submitting: true })
    try {
      const { id } = await api.bind(code, voice)
      toast(this, copy.bind.success)
      setTimeout(() => wx.redirectTo({ url: `/pages/kid/index?id=${encodeURIComponent(id)}` }), 900)
    } catch (e) {
      const msg = errorText(e, { 400: copy.bind.badCode, 409: copy.bind.taken, 429: copy.bind.tooMany })
      this.setData({ submitting: false, codeError: true })
      toast(this, msg, 'error')
    }
  },
})
