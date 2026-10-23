// 统一文案表：Web 上所有给小朋友与家长看的字都从这里取，便于审查语气。
// 规则：儿童友好、简短；不出现诱导留下地址、学校、电话、真实全名的措辞。

export const copy = {
  brand: { name: '可感', en: 'Palpable', slogan: '不只是玩偶，更是能说话的朋友', badge: 'Web 演示页面 · 黑客松版' },

  steps: [
    { key: 'unbox', title: '开箱', sub: '包装盒打开，取出玩偶放到桌面。', web: true },
    { key: 'name', title: '命名认主', sub: '设备第一次上线，向小朋友问好，请他起名；之后可以一对一聊天。', web: false },
    { key: 'park', title: '公园区域频道', sub: '场景切换到公园，询问是否加入区域频道。', web: false },
    { key: 'friend', title: '贴近成为好友', sub: '两个玩偶靠近，通过震动和提示音确认，建立好友关系。', web: false },
    { key: 'talk', title: '好友互相听见', sub: '好友说话，对方设备播放；好友进入频道时收到提醒。', web: false },
  ],

  link: { offline: '未连接', connecting: '连接中…', online: '已上线' } as const,

  light: {
    idle: '安静待命',
    listening: '正在听',
    thinking: '在想呢',
    speaking: '正在说',
    peer_talking: '好朋友在说话',
    muted: '暂时休息',
  } as const,

  unbox: {
    hint: '点一点盒子，打开看看',
    take: '把它抱出来',
    place: '放到桌面上',
    caption: '你好！我是可感，你可以给我起个名字吗？',
  },

  desk: {
    welcomeBubble: '你好呀！我是小可，你可以给我起个名字吗？',
    tapToTalk: '点一下玩偶，和它说话',
    micOn: '我在听，说吧～',
    micOff: '开始说话',
    micStop: '先不说了',
    toPark: '去公园玩',
    namedAs: (doll: string, kid: string) => `${kid}的好朋友「${doll}」`,
    unnamed: '还没有名字',
  },

  park: {
    title: '公园区域',
    askTitle: (zone: string) => `你已进入 ${zone}`,
    askSub: '要加入这个区域频道吗？',
    askHint: '对小伙伴说"加入"就好，也可以点下面的按钮假装说了',
    join: '加入',
    later: '暂不加入',
    membersTitle: '频道成员',
    membersSub: '（仅好友可见）',
    you: '你（我）',
    joined: '已加入',
    blocked: '已屏蔽',
    pending: '等家长确认',
    dragHint: '拖动玩偶在地图上走动，靠近好朋友试试',
    friendMade: '已成为好友！',
    friendPending: (name: string) => `和${name}贴在一起啦，等爸爸妈妈确认`,
    friendJoined: (name: string) => `${name} 已进入公园频道`,
    busy: '好朋友正在说，等一下哦',
  },

  friend: { someone: '一位好朋友' },

  device: {
    add: '再来一个小朋友',
    active: '麦克风在这只玩偶上',
    useMic: '用我的麦克风',
    errorUnsupported: '当前浏览器不支持 WebCodecs，请使用 Chrome / Edge',
    errorMic: (msg: string) => `无法打开麦克风：${msg}`,
  },

  parent: {
    title: '家长端',
    sub: '（Web 模拟）',
    tabs: { friends: '好友与群聊', digest: '今日回顾', control: '家长控制' },
    friends: '好友列表',
    groups: '所在群聊',
    noFriends: '还没有好朋友',
    online: '在线',
    offline: '离线',
    confirm: '确认',
    remove: '解除好友',
    pending: '待确认',
    digest: '今日回顾',
    chats: '对话',
    met: '遇到小朋友',
    shared: '分享的风景',
    noChats: '今天还没有聊天',
    controls: ['屏蔽好友', '解除好友', '对话摘要', '分享记录'],
    noKids: '名下还没有设备，打开玩偶上线后会自动出现',
    unit: { times: '次', people: '人' },
  },

  skins: {
    title: '设备皮套（不同风格）',
    sub: '同一个核心，换一个皮套，换一种个性。',
    list: [
      { skin: 'bear', name: '毛绒皮套', desc: '柔软亲和' },
      { skin: 'rabbit', name: '硅胶皮套', desc: '柔和光晕' },
      { skin: 'dragon', name: '恐龙皮套', desc: '活力有趣' },
      { skin: 'cat', name: '小猫皮套', desc: '机灵调皮' },
    ],
  },

  extras: {
    title: '其他功能',
    sub: '群聊、切换群、画面分享等更多功能，对小伙伴说出来就好。',
    items: [
      { title: '语音建群', say: '"创建群聊"', note: '小伙伴报出 4 位群码' },
      { title: '切换群', say: '"下一个群"', note: '灯光颜色跟着变' },
      { title: '画面分享', say: '"把看到的讲给好朋友听"', note: '小伙伴描述给好友听' },
    ],
  },
} as const

export type Copy = typeof copy
