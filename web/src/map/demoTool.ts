// 演示遥控：代替孩子对小伙伴说出指令（比如"加入"），服务端执行与 AI 调用相同的业务工具。
// 灵矽不可用时让演示继续；规则仍由服务端裁决。

export async function sayToCompanion(device: string, tool: string, args: Record<string, unknown> = {}): Promise<{ text: string; isError: boolean }> {
  const resp = await fetch('/world/tool', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ device, tool, arguments: args }),
  })
  if (!resp.ok) return { text: await resp.text(), isError: true }
  const out = (await resp.json()) as { text: string; is_error: boolean }
  return { text: out.text, isError: out.is_error }
}
