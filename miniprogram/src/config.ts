// 环境配置：按小程序版本（开发版 / 体验版 / 正式版）选服务端地址。
// 这里只有公开信息；AppSecret 只在服务端 config.yaml 里。

type EnvVersion = 'develop' | 'trial' | 'release'

interface EnvConfig {
  /** 家长端 REST 地址，须在小程序后台"request 合法域名"里登记（开发版可在开发者工具里关闭校验）。 */
  apiBase: string
  /**
   * 仅开发版：填服务端 demo.parent_token 时跳过微信登录，直接以演示家长身份调接口，
   * 便于没有 AppSecret 时本地联调。体验版、正式版不读取这个字段。
   */
  devToken?: string
}

const envs: Record<EnvVersion, EnvConfig> = {
  develop: { apiBase: 'http://127.0.0.1:8080', devToken: '' },
  trial: { apiBase: 'https://staging-api.palpable.example' },
  release: { apiBase: 'https://api.palpable.example' },
}

export const envVersion: EnvVersion = wx.getAccountInfoSync().miniProgram.envVersion
export const env: EnvConfig = envs[envVersion]

/** 法律文本与"关于"页用到的运营信息；上线前替换为真实主体与联系方式。 */
export const brand = {
  appName: '可感',
  company: '【运营主体全称】',
  address: '【注册地址】',
  email: '【客服邮箱】',
  version: '1.0.0',
  /** 隐私政策版本：政策更新时递增，家长需要重新同意。 */
  policyVersion: '2026-11-01',
}
