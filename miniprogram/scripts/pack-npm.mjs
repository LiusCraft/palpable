// 不开开发者工具也能构建 npm：把 node_modules 里的小程序组件包打到 src/miniprogram_npm。
import ci from 'miniprogram-ci'

const projectPath = new URL('..', import.meta.url).pathname
const result = await ci.packNpmManually({
  packageJsonPath: `${projectPath}package.json`,
  miniprogramNpmDistDir: `${projectPath}src/`,
})
console.log('packed', result)
