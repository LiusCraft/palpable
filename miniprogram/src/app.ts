import { installPrivacyHandler } from './services/privacy'

App({
  onLaunch() {
    installPrivacyHandler()
    // 新版本下载好后提示重启，避免家长长期停在旧版本的法律文本与接口上。
    const um = wx.getUpdateManager()
    um.onUpdateReady(() => {
      wx.showModal({
        title: '有新版本啦',
        content: '新版本已经准备好，重启一下就能用上',
        confirmText: '重启',
        success: (r) => r.confirm && um.applyUpdate(),
      })
    })
  },
})
