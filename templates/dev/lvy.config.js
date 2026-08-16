import {defineConfig} from 'lvyjs';
import {dirname, join} from 'path';
import {fileURLToPath} from 'url';
const __dirname = dirname(fileURLToPath(import.meta.url));
export default defineConfig({
  watch: ['src/**/*.{ts,tsx,js,jsx,json,html}'],
  alias: {
    entries: [{find: '@src', replacement: join(__dirname, 'src')}]
  },
  assets: {
    // 支持图片、字体、文本等静态资源
    filter: /\.(png|jpg|jpeg|gif|svg|webp|ico|yaml|txt|ttf|md)$/
  },
  build: {
    // JS 项目不经过 TypeScript 编译：旧版 lvyjs（rollup 插件）用
    // typescript: false 禁用 TS 插件，避免读取 JS 项目不存在的 tsconfig.json；
    // 新版 tsdown 引擎会忽略该字段，保留以兼容两种引擎。
    typescript: false,
    // 输出到 lib；入口默认 src 目录
    OutputOptions: {
      dir: 'lib'
    }
  }
});
