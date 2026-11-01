// 法律文本阅读页：?doc=terms|privacy|children
import { documents, type DocKey, type LegalDoc } from '../../legal/documents'

const keys: DocKey[] = ['terms', 'privacy', 'children']

Page({
  data: { doc: null as LegalDoc | null },
  onLoad(q: Record<string, string | undefined>) {
    const key = keys.find((k) => k === q.doc) ?? 'privacy'
    const doc = documents[key]
    wx.setNavigationBarTitle({ title: doc.title })
    this.setData({ doc })
  },
})
