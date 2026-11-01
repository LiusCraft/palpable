// 我的：称呼、法律文本、联系我们、退出登录与注销账号。
import { brand } from '../../config'
import { api } from '../../services/api'
import { copy, texts } from '../../utils/copy'
import { errorText, toast } from '../../utils/ui'

type Confirm = 'signOut' | 'delete' | ''

Page({
  data: {
    t: texts({ me: copy.me, common: copy.common }),
    version: copy.me.version(brand.version),
    company: brand.company,
    name: '',
    nameDialog: false,
    draft: '',
    confirm: '' as Confirm,
    confirmTitle: '',
    confirmBody: '',
    confirmText: '',
  },

  onShow() {
    api
      .me()
      .then((me) => this.setData({ name: me.display_name }))
      .catch((e) => toast(this, errorText(e), 'error'))
  },

  editName() {
    this.setData({ nameDialog: true, draft: this.data.name })
  },

  onDraft(e: WechatMiniprogram.CustomEvent<{ value: string }>) {
    this.setData({ draft: e.detail.value })
  },

  closeName() {
    this.setData({ nameDialog: false })
  },

  async saveName() {
    const name = this.data.draft.trim()
    if (!name || name.length > 20) return toast(this, copy.me.nameInvalid, 'warning')
    try {
      await api.setDisplayName(name)
      this.setData({ name, nameDialog: false })
      toast(this, copy.me.nameSaved)
    } catch (e) {
      toast(this, errorText(e, { 400: copy.me.nameInvalid }), 'error')
    }
  },

  openDoc(e: WechatMiniprogram.TouchEvent) {
    wx.navigateTo({ url: `/pages/legal/index?doc=${e.currentTarget.dataset.doc}` })
  },

  askSignOut() {
    this.setData({ confirm: 'signOut', confirmTitle: copy.me.signOut, confirmBody: '', confirmText: copy.me.signOut })
  },

  askDelete() {
    this.setData({
      confirm: 'delete',
      confirmTitle: copy.me.deleteTitle,
      confirmBody: copy.me.deleteBody,
      confirmText: copy.me.deleteConfirm,
    })
  },

  closeConfirm() {
    this.setData({ confirm: '' })
  },

  async onConfirm() {
    const what = this.data.confirm
    this.setData({ confirm: '' })
    try {
      if (what === 'delete') await api.deleteAccount()
      else await api.signOut()
      toast(this, what === 'delete' ? copy.me.deleted : copy.me.signOutDone)
      // 回到首页会静默以新会话登录；注销后是一个全新的空账号。
      setTimeout(() => wx.reLaunch({ url: '/pages/home/index' }), 900)
    } catch (e) {
      toast(this, errorText(e), 'error')
    }
  },
})
