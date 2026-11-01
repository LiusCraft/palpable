// 6 位绑定码输入：一排大格子，背后是一个隐藏的数字输入框。
const LENGTH = 6

Component({
  properties: {
    error: { type: Boolean, value: false },
  },
  data: {
    value: '',
    cells: Array.from({ length: LENGTH }, () => ''),
    focused: true,
  },
  methods: {
    onInput(e: WechatMiniprogram.Input) {
      const value = e.detail.value.replace(/\D/g, '').slice(0, LENGTH)
      const cells = Array.from({ length: LENGTH }, (_, i) => value[i] ?? '')
      this.setData({ value, cells })
      this.triggerEvent('change', { value, complete: value.length === LENGTH })
      return value
    },
    focus() {
      this.setData({ focused: true })
    },
    onBlur() {
      this.setData({ focused: false })
    },
    clear() {
      this.setData({ value: '', cells: Array.from({ length: LENGTH }, () => ''), focused: true })
      this.triggerEvent('change', { value: '', complete: false })
    },
  },
})
