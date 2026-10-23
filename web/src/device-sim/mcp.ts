// 设备端 MCP 服务：实现设备自带的工具（灯光、振动、拍照）。工具名形如 self.<模块>.<功能>，
// 由服务端代理与业务工具合并后交给 AI。这里只执行设备动作，不含业务判断。

export interface JsonRpc {
  jsonrpc: '2.0'
  id?: number | string
  method?: string
  params?: Record<string, unknown>
  result?: unknown
  error?: { code: number; message: string }
}

export interface DeviceActions {
  setLight: (color: string, pattern: 'solid' | 'breathe' | 'pulse') => void
  haptic: (pattern: 'tap' | 'double_tap' | 'long') => void
  /** Web 用当前场景代替摄像头：返回眼前画面的文字描述。 */
  look: () => string
}

const COLOR_NAMES: Record<string, string> = {
  红: '#ff5a5f', 红色: '#ff5a5f', red: '#ff5a5f',
  橙: '#ff9f43', 橙色: '#ff9f43', orange: '#ff9f43',
  黄: '#feca57', 黄色: '#feca57', yellow: '#feca57',
  绿: '#4caf50', 绿色: '#4caf50', green: '#4caf50',
  蓝: '#54a0ff', 蓝色: '#54a0ff', blue: '#54a0ff',
  紫: '#a55eea', 紫色: '#a55eea', purple: '#a55eea',
  粉: '#ff6b9d', 粉色: '#ff6b9d', pink: '#ff6b9d',
  白: '#ffffff', 白色: '#ffffff', white: '#ffffff',
}

const TOOLS = [
  {
    name: 'self.light.set',
    description: '把玩偶的灯带换个颜色，比如开心时变粉色。color 可以是颜色名（红、橙、黄、绿、蓝、紫、粉、白）或 #RRGGBB。',
    inputSchema: {
      type: 'object',
      properties: {
        color: { type: 'string' },
        pattern: { type: 'string', enum: ['solid', 'breathe', 'pulse'] },
      },
      required: ['color'],
    },
  },
  {
    name: 'self.haptic.pulse',
    description: '让玩偶轻轻振动一下，像抱抱或者拍拍小朋友。',
    inputSchema: { type: 'object', properties: { pattern: { type: 'string', enum: ['tap', 'double_tap', 'long'] } } },
  },
  {
    name: 'self.camera.take_photo',
    description: '看一看玩偶眼前的画面，返回画面的文字描述。分享画面前先调用它。',
    inputSchema: { type: 'object', properties: {} },
  },
]

function text(t: string, isError = false) {
  return { content: [{ type: 'text', text: t }], isError }
}

/** 处理一条来自服务端（代理）的 JSON-RPC 请求，返回要回给服务端的回复；通知返回 null。 */
export function handleMcp(req: JsonRpc, act: DeviceActions): JsonRpc | null {
  if (req.id === undefined) return null
  const reply = (result: unknown): JsonRpc => ({ jsonrpc: '2.0', id: req.id, result })
  switch (req.method) {
    case 'initialize':
      return reply({
        protocolVersion: '2024-11-05',
        capabilities: { tools: {} },
        serverInfo: { name: 'palpable-web-sim', version: '0.2.0' },
      })
    case 'tools/list':
      return reply({ tools: TOOLS, nextCursor: '' })
    case 'tools/call': {
      const name = String(req.params?.name ?? '')
      const args = (req.params?.arguments ?? {}) as Record<string, unknown>
      if (name === 'self.light.set') {
        const raw = String(args.color ?? '').trim()
        const color = /^#[0-9a-fA-F]{6}$/.test(raw) ? raw : COLOR_NAMES[raw.toLowerCase()]
        if (!color) return reply(text('不认识这个颜色，换一个试试吧。', true))
        const pattern = args.pattern === 'pulse' || args.pattern === 'solid' ? args.pattern : 'breathe'
        act.setLight(color, pattern)
        return reply(text('灯光换好啦。'))
      }
      if (name === 'self.haptic.pulse') {
        const p = args.pattern === 'tap' || args.pattern === 'long' ? args.pattern : 'double_tap'
        act.haptic(p)
        return reply(text('轻轻振动了一下。'))
      }
      if (name === 'self.camera.take_photo') return reply(text(act.look()))
      return { jsonrpc: '2.0', id: req.id, error: { code: -32601, message: `unknown tool ${name}` } }
    }
    default:
      return { jsonrpc: '2.0', id: req.id, error: { code: -32601, message: 'method not found' } }
  }
}
