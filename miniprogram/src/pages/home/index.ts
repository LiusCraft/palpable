// 首页：名下的小伙伴。未同意隐私政策前不登录、不请求。
import { api } from '../../services/api'
import { hasAgreed } from '../../services/privacy'
import type { Kid } from '../../services/types'
import { copy, texts } from '../../utils/copy'
import { skinLook } from '../../utils/skins'
import { greeting } from '../../utils/time'
import { errorText } from '../../utils/ui'

interface KidView extends Kid {
  tint: string
  owner: string
  /** 角标：等你同意的请求优先，其次是起名阶段 */
  badge: string
  badgeAlert: boolean
}

function view(k: Kid): KidView {
  const stage = k.stage === 'fresh' ? copy.home.stageFresh : k.stage === 'named' ? copy.home.stageNamed : ''
  return {
    ...k,
    doll_name: k.doll_name || skinLook(k.skin).label,
    tint: skinLook(k.skin).tint,
    owner: copy.home.ownerOf(k.kid_name),
    badge: k.waiting_you ? copy.home.waiting(k.waiting_you) : stage,
    badgeAlert: k.waiting_you > 0,
  }
}

Page({
  data: {
    t: texts({ home: copy.home, common: copy.common }),
    topPad: 0,
    navHeight: 44,
    greeting: '',
    name: '',
    kids: [] as KidView[],
    loading: true,
    failed: '',
    needPrivacy: false,
  },

  onLoad() {
    // 自定义导航栏：标题与胶囊按钮对齐。
    const win = wx.getWindowInfo()
    const menu = wx.getMenuButtonBoundingClientRect()
    const top = win.statusBarHeight || 20
    this.setData({ topPad: top, navHeight: (menu.top - top) * 2 + menu.height })
  },

  onShow() {
    this.setData({ greeting: greeting() })
    if (!hasAgreed()) {
      this.setData({ needPrivacy: true, loading: false })
      return
    }
    this.load()
  },

  onPullDownRefresh() {
    this.load().finally(() => wx.stopPullDownRefresh())
  },

  async load() {
    if (!hasAgreed()) return
    this.setData({ failed: '' })
    try {
      const [kids, me] = await Promise.all([api.kids(), api.me().catch(() => null)])
      this.setData({ kids: kids.map(view), name: me?.display_name ?? '', loading: false })
    } catch (e) {
      this.setData({ loading: false, failed: errorText(e) })
    }
  },

  onPrivacyAgree() {
    this.setData({ needPrivacy: false, loading: true })
    this.load()
  },

  onPrivacyDisagree() {
    this.setData({ needPrivacy: false })
  },

  onBindTap() {
    if (hasAgreed()) this.bind()
    else this.setData({ needPrivacy: true })
  },

  openKid(e: WechatMiniprogram.TouchEvent) {
    wx.navigateTo({ url: `/pages/kid/index?id=${encodeURIComponent(e.currentTarget.dataset.id as string)}` })
  },

  bind() {
    wx.navigateTo({ url: '/pages/bind/index' })
  },

  openMe() {
    wx.navigateTo({ url: '/pages/me/index' })
  },
})
