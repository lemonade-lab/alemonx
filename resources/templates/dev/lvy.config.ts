import { defineConfig } from 'lvyjs';
import { dirname, join } from 'path';
import { fileURLToPath } from 'url';
import postcss from 'lvyjs-plugin-postcss';
import tailwindcss from 'lvyjs-plugin-tailwindcss';
// https://github.com/lemonade-lab/lvyjs#readme
export default defineConfig({
  plugins: [
    postcss({
      include: /\.(scss|less)$/,
    }),
    tailwindcss()
  ],
  watch: ['src/**/*.{ts,tsx,js,jsx,json,html}'],
  alias: {
    entries: [
      {
        find: '@src',
        replacement: join(dirname(fileURLToPath(import.meta.url)), 'src')
      }
    ]
  },
  assets: {
    // 将以下文件类型视为静态资源
    include: /\.(png|jpg|jpeg|gif|svg|webp|ico|yaml|txt|ttf|md)$/
  }
});
