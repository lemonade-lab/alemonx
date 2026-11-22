import DOMPurify from 'dompurify'
import hljs from 'highlight.js/lib/core'
import bash from 'highlight.js/lib/languages/bash'
import css from 'highlight.js/lib/languages/css'
import javascript from 'highlight.js/lib/languages/javascript'
import json from 'highlight.js/lib/languages/json'
import markdown from 'highlight.js/lib/languages/markdown'
import ini from 'highlight.js/lib/languages/ini'
import typescript from 'highlight.js/lib/languages/typescript'
import xml from 'highlight.js/lib/languages/xml'
import yaml from 'highlight.js/lib/languages/yaml'

const highlighter = hljs.newInstance()
for (const [name, grammar] of Object.entries({
  bash, css, javascript, json, markdown, ini, typescript, xml, yaml
})) {
  highlighter.registerLanguage(name, grammar)
}
highlighter.registerLanguage('ignore', () => ({
  contains: [
    { scope: 'comment', begin: /^#/, end: /$/ },
    { scope: 'keyword', match: /^!/ }
  ]
}))
highlighter.registerLanguage('settings', grammar => ({
  contains: [
    grammar.COMMENT(/^[ \t]*[#;]/, /$/),
    {
      begin: [/^[ \t]*/, /(?:export[ \t]+)?/, /[^\s=;#]+/, /(?=[ \t]*=)/],
      beginScope: { 2: 'keyword', 3: 'attr' },
      starts: {
        end: /$/,
        contains: [grammar.QUOTE_STRING_MODE, grammar.APOS_STRING_MODE, grammar.NUMBER_MODE]
      }
    }
  ]
}))

const languageByExtension = new Map(Object.entries({
  js: 'javascript', jsx: 'javascript', cjs: 'javascript', mjs: 'javascript',
  ts: 'typescript', tsx: 'typescript', cts: 'typescript', mts: 'typescript',
  json: 'json', jsonc: 'json', yaml: 'yaml', yml: 'yaml', toml: 'ini', ini: 'ini',
  md: 'markdown', markdown: 'markdown', sh: 'bash', bash: 'bash', css: 'css',
  html: 'xml', htm: 'xml', xml: 'xml', svg: 'xml', env: 'settings'
}))

export function highlightFileSource(file: string, content: string) {
  // Highlighting runs on the editor thread, so keep it bounded for large files.
  if (content.length > 64 * 1024) return null
  const name = file.toLowerCase().split(/[\\/]/).at(-1) ?? ''
  const extension = name.includes('.') ? name.split('.').at(-1) : undefined
  const language = name === '.gitignore' || name === '.dockerignore'
    ? 'ignore'
    : name === '.npmrc' || name.startsWith('.env.')
      ? 'settings'
      : languageByExtension.get(extension ?? '')
  if (!language) return null

  const highlighted = highlighter.highlight(content, { language }).value
  return DOMPurify.sanitize(highlighted, {
    ALLOWED_TAGS: ['span'],
    ALLOWED_ATTR: ['class']
  })
}
