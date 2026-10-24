// 统一文案表：Web 上所有给小朋友与家长看的字都从这里取，便于审查语气。
// 规则：儿童友好、简短；不出现诱导留下地址、学校、电话、真实全名的措辞。

export const copy = {
  brand: { name: '可感', en: 'Palpable', slogan: '不只是玩偶，更是能说话的朋友', badge: 'Web 演示页面 · 黑客松版' },

  // 官网首页（给家长和评委看的）。
  site: {
    nav: [
      { id: 'top', label: '首页' },
      { id: 'features', label: '产品特点' },
      { id: 'how', label: '使用场景' },
      { id: 'about', label: '关于我们' },
    ],
    start: '开始体验',
    title: '让孩子的世界',
    title2: '多一个会说话的朋友',
    lead: '以语音为主要交互，外观可被玩偶皮套重新定义的\nAI 对讲机。',
    navigationLabel: '网站导航',
    openMenu: '打开导航',
    closeMenu: '收起导航',
    skipToContent: '跳到产品介绍',
    notes: { hello: '你好！我是你的新朋友！', slogan: '不只是玩偶，更是能说话的朋友', helloLines: ['你好！', '我是你的新朋友！'], sloganLines: ['不只是玩偶，', '更是能说话的朋友'] },
    signs: ['一起玩', '一起探索', '一起成长'],
    features: [
      { key: 'voice', title: '语音对话', text: '贴近、唤醒、说话\n简单三步，轻松开聊', tone: '#dcecff', fg: '#2588ff' },
      { key: 'friends', title: '好友与群聊', text: '靠近即连，语音建群\n和小伙伴一起玩', tone: '#e3dfff', fg: '#7764ff' },
      { key: 'zone', title: '区域频道', text: '进入公园等区域\n与同区域的好友互动', tone: '#e1f3d9', fg: '#4eb44d' },
      { key: 'safe', title: '默认安全', text: '陌生人屏蔽，家长可控\n为孩子的社交保驾护航', tone: '#fff1d6', fg: '#ffad08' },
      { key: 'skin', title: '可换皮套', text: '一机多角色，毛绒 / 硅胶 / 硬壳\n让每个孩子都有自己的风格', tone: '#ffdee7', fg: '#f16886' },
    ],
    more: ['探索更多', '精彩功能'],
    ctaPlay: '观看演示',
    ctaParent: '看看家长端',
    requirement: '体验需要 Chrome 或 Edge，并允许使用麦克风。',
    actions: {
      title: '孩子只做三件事',
      sub: '贴近、唤醒、说话。没有账号，没有界面，一个玩偶就是入口。',
      items: [
        { icon: '🤝', title: '贴近', text: '两个玩偶靠近，震动和提示音告诉孩子“关联上了”，他们就成了好朋友。' },
        { icon: '✨', title: '唤醒', text: '抱起来、摸一摸，小伙伴就醒了。灯带亮起来，告诉孩子它在听。' },
        { icon: '🎙️', title: '说话', text: '不用按住，直接开口。孩子说话，小伙伴回答；和好友在同一个频道时，说的话会传给好友。' },
      ],
    },
    how: {
      eyebrow: '一起玩，一起探索，一起成长',
      title: '一天的玩法',
      sub: '从拆开盒子，到和新朋友在公园里聊起来。',
      cta: '进房间，亲手开箱',
      steps: [
        { title: '开箱认主', text: '打开盒子，抱出玩偶放上桌面。小可会打招呼，请孩子给小伙伴起个名字。' },
        { title: '公园频道', text: '走进公园，小伙伴会问要不要加入区域频道，孩子同意才会加入。' },
        { title: '贴一贴成好友', text: '和另一个小朋友的玩偶走到一起，就成为好友，家长会收到通知。' },
        { title: '好友互相听见', text: '只有好友能互相听见，陌生小朋友在同一个频道里也被屏蔽。' },
        { title: '家长随时看', text: '家长端能看好友、群聊和今日回顾，随时解除或屏蔽好友。' },
      ],
    },
    why: {
      title: '为什么不是手机或微信语音',
      sub: '四点，缺一不可。',
      items: [
        { title: '孩子不需要手机', text: '对低龄儿童和老人，一个玩偶就是入口，没有账号、没有界面。' },
        { title: '物理入口', text: '抱着就能说，靠近就能连，不需要找 App、翻列表。' },
        { title: 'AI 是群里的成员，不是插件', text: '它记得孩子，会陪聊，会在朋友加入时提醒。' },
        { title: '默认安全', text: '陌生设备互相屏蔽，好友关系家长看得见、管得住。通用社交软件做不到对儿童的这种默认边界。' },
      ],
    },
    safe: {
      title: '让家长放心的五件事',
      sub: '规则都在服务端，设备端绕不过去。',
      items: [
        { icon: '🛡️', title: '陌生人无法接触孩子', text: '没建立好友关系的设备，声音不会被转发，也不会出现在对方的在线列表里。' },
        { icon: '👩', title: '家长看得见，管得住', text: '新朋友会通知家长；家长端能看好友、群聊和对话摘要，随时解除或屏蔽。也可以设成每个新好友先经家长确认。' },
        { icon: '🔏', title: '声纹最小化', text: '只保存声纹特征与绑定关系，不长期保留原始音频；删除设备时一并删除声纹。' },
        { icon: '💬', title: '内容友好', text: 'AI 用儿童友好的语气说话，不诱导孩子说出地址、学校、电话等个人信息。' },
        { icon: '⚙️', title: '规则不在设备上', text: '好友、频道、屏蔽都由服务端裁决，玩偶本身只是传感器和喇叭。' },
      ],
      note: '说明：引导对话的音频会在服务商侧保留约 60 天，仅用于登记声纹，需要家长同意，也可以跳过。未成年人的声纹属于敏感个人信息，正式上线前会落实监护人单独同意、明确的保存期限与删除机制。',
    },
    about: { title: '不只是玩偶，更是能说话的朋友', text: '我们想让科技离屏幕远一点，离陪伴近一点。可感把 AI 语音装进可以拥抱的小伙伴里，让孩子自在开口，也和真正的好朋友连在一起。' },
    art: { title: '四个等着认识你的毛绒小伙伴', description: '小兔、小熊、小恐龙和熊猫坐在湖边的石头上，脖子上亮着粉色、蓝色和绿色的灯环。' },
    safetyLink: '隐私与安全',
    final: { title: '来看看吧', sub: '不用安装，打开浏览器，就能走进小熊的房间。' },
    footer: '黑客松版，功能以实际体验为准',
  },

  link: { offline: '未连接', connecting: '连接中…', online: '已上线' } as const,

  light: {
    idle: '安静待命',
    listening: '正在听',
    thinking: '在想呢',
    speaking: '正在说',
    peer_talking: '好朋友在说话',
    muted: '暂时休息',
  } as const,

  room: {
    boxed: '走到盒子旁边，按 E 打开它',
    open: '玩偶在里面！按 E 把它抱出来',
    held: '走到桌子前的小圈圈，按 E 把玩偶放下',
    placedUnnamed: '直接开口和它说话吧，不用按任何键',
    placedNamed: '想去公园？走到桌子前按 E 抱起玩偶，再走到门口按 E',
    openBox: '打开盒子',
    takeOut: '把玩偶抱出来',
    putDown: '把玩偶放到桌上',
    pickUp: '抱起玩偶',
    goOut: '出门去公园',
    goHome: '回家',
    needHold: '出门前，先抱起玩偶哦',
    gate: '就选它，走进房间',
    choose: '先选一只玩偶',
    chooseSub: '点一点，看看喜欢哪一只；它会装在盒子里等你',
    gateBack: '欢迎回来，点一下继续',
    lockHint: '点一下画面，用鼠标看四周（Esc 放开鼠标）',
    listening: '在听你说话',
    micRetry: '打开麦克风',
  },

  park: {
    title: '公园区域',
    askTitle: (zone: string) => `你已进入 ${zone}`,
    askSub: '要加入这个区域频道吗？',
    askHint: '直接对小伙伴说“加入”就好',
    join: '加入',
    later: '暂不加入',
    membersTitle: '频道成员',
    membersSub: '（仅好友可见）',
    you: '你（我）',
    joined: '已加入',
    blocked: '已屏蔽',
    pending: '等家长确认',
    walkHint: 'W A S D 走路，移动鼠标看四周，走到小伙伴身边贴一贴；按 F 摸一摸玩偶；想回家，走到家门口按 E',
    friendMade: (name: string) => `和${name}贴在一起，成为好朋友啦！`,
    friendPending: (name: string) => `和${name}贴在一起啦，等爸爸妈妈确认`,
    friendJoined: (name: string) => `${name} 已进入公园频道`,
    busy: '好朋友正在说，等一下哦',
  },

  friend: { someone: '一位好朋友' },

  device: {
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
    autoAdded: '贴一贴成为了好朋友，不满意可在“家长控制”解除',
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
