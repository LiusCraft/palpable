// 家长端 REST 的响应类型，与 server/internal/parentapi 的 JSON 一一对应。

export type Stage = 'fresh' | 'named' | 'ready'

export interface Kid {
  id: string
  doll_name: string
  kid_name: string
  skin: string
  stage: Stage
  online: boolean
  /** 等这位家长批准的交友请求数 */
  waiting_you: number
}

export interface Friend {
  request_id: string
  device_id: string
  name: string
  status: 'pending' | 'confirmed'
  /** pending 时：you 等你同意；other 你已同意（或自动放行），在等对方家长 */
  waiting_for?: 'you' | 'other'
  auto: boolean
  agent_blocked: boolean
  online: boolean
  since: string
}

export interface Group {
  id: string
  kind: 'zone' | 'group'
  name: string
  code?: string
  expires_at?: string
  foreground: boolean
  members: number
}

export type FriendPolicy = 'approve' | 'notify'

/** GET /settings */
export interface Settings {
  friend_policy: FriendPolicy
  agent_in_channel: boolean
  voice_consent: boolean
  /** 已经登记了声纹 */
  voice_enrolled: boolean
  /** 还能补开（登记只在小伙伴起名前进行） */
  voice_can_enable: boolean
}

/** PUT /settings；voice_consent 省略表示不改 */
export interface SettingsUpdate {
  friend_policy: FriendPolicy
  agent_in_channel: boolean
  voice_consent?: boolean
}

export interface Digest {
  met: { friend: string; name: string; channel: string; at: string }[] | null
  shared: { from: string; name: string; text: string; channel: string; at: string }[] | null
  chats: { role: 'user' | 'assistant'; text: string; at: string }[] | null
}

export interface Me {
  id: string
  display_name: string
  created_at: string
}

export interface SessionToken {
  token: string
  expires_at: string
}
