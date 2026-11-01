// 孩子详情：今天、朋友、设置。好友放行、解除、屏蔽都由服务端执行规则，这里只发请求并展示结果。
import { api } from '../../services/api'
import type { Digest, Friend, FriendPolicy, Group, Kid, Settings, SettingsUpdate } from '../../services/types'
import { copy, texts } from '../../utils/copy'
import { skinLook } from '../../utils/skins'
import { clock, relativeDay } from '../../utils/time'
import { errorText, toast } from '../../utils/ui'

type Tab = 'today' | 'friends' | 'settings'
type DialogAction = 'remove' | 'decline' | 'unbind' | 'voiceOff' | ''

interface FriendView extends Friend {
  sinceText: string
}

interface GroupView extends Group {
  label: string
  membersText: string
}

interface DigestView {
  met: { name: string; time: string }[]
  shared: { name: string; text: string; time: string }[]
  chats: { kid: boolean; text: string; time: string }[]
}

const emptyDigest: DigestView = { met: [], shared: [], chats: [] }

function digestView(d: Digest): DigestView {
  return {
    met: (d.met ?? []).map((m) => ({ name: m.name, time: clock(m.at) })),
    shared: (d.shared ?? []).map((s) => ({ name: s.name, text: s.text, time: clock(s.at) })),
    chats: (d.chats ?? []).map((c) => ({ kid: c.role === 'user', text: c.text, time: clock(c.at) })),
  }
}

/** 声音登记开关下的说明 */
function voiceHint(s: Settings): string {
  if (s.voice_consent) return s.voice_enrolled ? copy.kid.voiceOnEnrolled : copy.kid.voiceOnPending
  return s.voice_can_enable ? copy.kid.voiceOffCan : copy.kid.voiceOffLocked
}

function groupView(g: Group): GroupView {
  return {
    ...g,
    label: g.kind === 'group' && g.code ? copy.kid.group(g.code) : copy.kid.zone,
    membersText: copy.kid.members(g.members),
  }
}

Page({
  data: {
    t: texts({ kid: copy.kid, common: copy.common }),
    id: '',
    tab: 'today' as Tab,
    kid: null as Kid | null,
    dollName: '',
    owner: '',
    tint: '#FFF3D6',
    loading: true,
    digest: emptyDigest,
    groups: [] as GroupView[],
    askYou: [] as FriendView[],
    askOther: [] as FriendView[],
    friends: [] as FriendView[],
    settings: {
      friend_policy: 'approve',
      agent_in_channel: true,
      voice_consent: false,
      voice_enrolled: false,
      voice_can_enable: false,
    } as Settings,
    voiceHint: '',
    busy: '',
    dialog: { visible: false, title: '', content: '', confirm: '', action: '' as DialogAction, friend: '' },
  },

  onLoad(q: Record<string, string | undefined>) {
    this.setData({ id: decodeURIComponent(q.id ?? '') })
  },

  onShow() {
    this.load()
  },

  onPullDownRefresh() {
    this.load().finally(() => wx.stopPullDownRefresh())
  },

  async load() {
    const id = this.data.id
    try {
      const [kids, friends, groups, digest, settings] = await Promise.all([
        api.kids(),
        api.friends(id),
        api.groups(id),
        api.digest(id),
        api.settings(id),
      ])
      const kid = kids.find((k) => k.id === id)
      if (!kid) {
        this.gone()
        return
      }
      const look = skinLook(kid.skin)
      const dollName = kid.doll_name || look.label
      wx.setNavigationBarTitle({ title: dollName })
      this.setData({
        kid,
        dollName,
        owner: copy.home.ownerOf(kid.kid_name),
        tint: look.tint,
        digest: digestView(digest),
        groups: groups.map(groupView),
        settings,
        voiceHint: voiceHint(settings),
        loading: false,
        ...this.splitFriends(friends),
      })
    } catch (e) {
      this.setData({ loading: false })
      toast(this, errorText(e, { 404: copy.errors.notFound }), 'error')
    }
  },

  splitFriends(all: Friend[]) {
    const view = (f: Friend): FriendView => ({ ...f, name: f.name || '新朋友', sinceText: copy.kid.since(relativeDay(f.since)) })
    return {
      askYou: all.filter((f) => f.status === 'pending' && f.waiting_for === 'you').map(view),
      askOther: all.filter((f) => f.status === 'pending' && f.waiting_for !== 'you').map(view),
      friends: all.filter((f) => f.status === 'confirmed').map(view),
    }
  },

  async refreshFriends() {
    const friends = await api.friends(this.data.id)
    this.setData(this.splitFriends(friends))
    return friends
  },

  gone() {
    toast(this, copy.errors.notFound, 'warning')
    setTimeout(() => wx.navigateBack({ fail: () => wx.reLaunch({ url: '/pages/home/index' }) }), 1200)
  },

  onTab(e: WechatMiniprogram.CustomEvent<{ value: Tab }>) {
    this.setData({ tab: e.detail.value })
  },

  // ---- 朋友 ----

  async approve(e: WechatMiniprogram.TouchEvent) {
    const req = e.currentTarget.dataset.req as string
    if (this.data.busy) return
    this.setData({ busy: req })
    try {
      await api.confirm(req)
      const friends = await this.refreshFriends()
      const still = friends.find((f) => f.request_id === req && f.status === 'pending')
      toast(this, still ? copy.kid.approvedBoth : copy.kid.approved)
    } catch (err) {
      toast(this, errorText(err), 'error')
      this.refreshFriends().catch(() => undefined)
    } finally {
      this.setData({ busy: '' })
    }
  },

  decline(e: WechatMiniprogram.TouchEvent) {
    const { device, name } = e.currentTarget.dataset as { device: string; name: string }
    this.ask('decline', device, copy.kid.declineTitle(name), copy.kid.declineBody, copy.kid.decline)
  },

  friendMenu(e: WechatMiniprogram.TouchEvent) {
    const f = this.data.friends.find((x) => x.device_id === e.currentTarget.dataset.device)
    if (!f) return
    const a = copy.kid.friendActions
    wx.showActionSheet({
      itemList: [f.agent_blocked ? a.unblock : a.block, a.remove],
      itemColor: '#4A3B32',
      success: (r) => {
        if (r.tapIndex === 0) this.toggleAgent(f)
        else this.ask('remove', f.device_id, copy.kid.removeTitle(f.name), copy.kid.removeBody, a.remove)
      },
    })
  },

  async toggleAgent(f: FriendView) {
    try {
      await api.blockAgent(this.data.id, f.device_id, !f.agent_blocked)
      await this.refreshFriends()
      toast(this, copy.kid.saved)
    } catch (err) {
      toast(this, errorText(err), 'error')
    }
  },

  // ---- 设置 ----

  async onPolicy(e: WechatMiniprogram.CustomEvent<{ value: FriendPolicy }>) {
    await this.save({ friend_policy: e.detail.value })
  },

  async onAgent(e: WechatMiniprogram.CustomEvent<{ value: boolean }>) {
    await this.save({ agent_in_channel: e.detail.value })
  },

  onVoice(e: WechatMiniprogram.CustomEvent<{ value: boolean }>) {
    if (e.detail.value) {
      this.save({ voice_consent: true })
      return
    }
    // 关闭会立即删除声音特征，先确认；开关先保持原样，确认后再变。
    this.setData({ 'settings.voice_consent': true })
    this.ask('voiceOff', '', copy.kid.voiceOffTitle, copy.kid.voiceOffBody, copy.kid.voiceOffConfirm)
  },

  /** 只改一项；服务端规则决定能不能改，失败时恢复原样。 */
  async save(change: Partial<SettingsUpdate>) {
    const prev = this.data.settings
    const body: SettingsUpdate = { friend_policy: prev.friend_policy, agent_in_channel: prev.agent_in_channel, ...change }
    const next: Settings = { ...prev, ...change }
    this.setData({ settings: next })
    try {
      await api.saveSettings(this.data.id, body)
      if (change.voice_consent === false) next.voice_enrolled = false
      if (change.voice_consent !== undefined) this.setData({ settings: next, voiceHint: voiceHint(next) })
      toast(this, change.voice_consent === false ? copy.kid.voiceDeleted : copy.kid.saved)
    } catch (err) {
      this.setData({ settings: prev })
      toast(this, errorText(err, { 409: copy.kid.voiceTooLate, 503: copy.kid.voiceUnavailable }), 'error')
    }
  },

  unbind() {
    this.ask('unbind', '', copy.kid.unbindTitle, copy.kid.unbindBody(this.data.dollName), copy.kid.unbindConfirm)
  },

  copyCode(e: WechatMiniprogram.TouchEvent) {
    const code = e.currentTarget.dataset.code as string
    if (code) wx.setClipboardData({ data: code })
  },

  // ---- 确认弹窗 ----

  ask(action: DialogAction, friend: string, title: string, content: string, confirm: string) {
    this.setData({ dialog: { visible: true, title, content, confirm, action, friend } })
  },

  closeDialog() {
    this.setData({ 'dialog.visible': false })
  },

  async onDialogConfirm() {
    const { action, friend } = this.data.dialog
    this.closeDialog()
    if (action === 'voiceOff') {
      await this.save({ voice_consent: false })
      return
    }
    try {
      if (action === 'unbind') {
        await api.unbind(this.data.id)
        toast(this, copy.kid.unbound)
        setTimeout(() => wx.navigateBack({ fail: () => wx.reLaunch({ url: '/pages/home/index' }) }), 900)
        return
      }
      await api.removeFriend(this.data.id, friend)
      await this.refreshFriends()
      toast(this, action === 'decline' ? copy.kid.declined : copy.kid.removed)
    } catch (err) {
      toast(this, errorText(err), 'error')
    }
  },
})
