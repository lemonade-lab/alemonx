import { useStoreState } from '../store/guideStore'
import { type ReactNode } from 'react'
import Markdown from 'markdown-to-jsx'
import { Check, Copy } from 'lucide-react'
import { highlightCode } from './highlight'

// 从代码块内容提取语言标签（```tsx 等第一行）。
function detectLanguage(content: string): string {
  const firstLine = content.split('\n')[0] ?? ''
  const match = firstLine.trim().match(/^```([a-zA-Z0-9_+-]+)/)
  return match ? match[1] : ''
}

// CodeBlock 渲染带语言标签、复制按钮和语法高亮的代码块。markdown-to-jsx 把
// ```lang 的代码传给 <pre>，children 是 <code>。这里接管 pre 的渲染。
function CodeBlock({
  children,
  streaming
}: {
  children: ReactNode
  streaming?: boolean
}) {
  const [copied, setCopied] = useStoreState(false)
  // markdown-to-jsx 的 pre 里是 <code>，其文本即代码。
  let codeText = ''
  const extract = (node: ReactNode) => {
    if (node === null || node === undefined) return
    if (typeof node === 'string') {
      codeText += node
    } else if (Array.isArray(node)) {
      node.forEach(extract)
    } else if (typeof node === 'object' && 'props' in (node as object)) {
      const element = node as { props?: { children?: ReactNode } }
      extract(element.props?.children)
    }
  }
  extract(children)
  const language = detectLanguage(codeText)

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(
        codeText
          .replace(/^```[a-zA-Z0-9_+-]*\s*\n?/, '')
          .replace(/\n```\s*$/, '')
      )
      setCopied(true)
      setTimeout(() => setCopied(false), 1800)
    } catch {
      // 复制失败不打扰
    }
  }

  const highlighted = highlightCode(
    codeText.replace(/^```[a-zA-Z0-9_+-]*\s*\n?/, '').replace(/\n```\s*$/, ''),
    language
  )

  return (
    <div className="my-3 overflow-hidden rounded-[10px] border border-(--theme-border-subtle) bg-(--theme-surface-code) [&_.tok-comment]:italic [&_.tok-comment]:text-[#7f848e] [&_.tok-function]:text-[#61afef] [&_.tok-keyword]:text-[#c678dd] [&_.tok-number]:text-[#d19a66] [&_.tok-string]:text-[#98c379]">
      <div className="flex items-center justify-between border-b border-(--theme-border-subtle) bg-[color-mix(in_srgb,var(--theme-surface-code)_90%,var(--theme-surface-hover)_10%)] px-3 py-[5px]">
        {language ? (
          <span className="font-mono text-[0.68rem] font-semibold tracking-[0.02em] text-(--theme-text-muted) uppercase">
            {language}
          </span>
        ) : (
          <span className="font-mono text-[0.68rem] font-semibold tracking-[0.02em] text-(--theme-text-muted) uppercase">
            代码
          </span>
        )}
        <div className="flex items-center gap-2.5">
          {streaming && (
            <span className="inline-flex items-center gap-[5px] text-[0.66rem] text-(--theme-accent-text) before:size-1.5 before:animate-agent-pulse before:rounded-full before:bg-(--theme-accent) before:content-['']">
              生成中…
            </span>
          )}
          <button
            className="inline-flex cursor-pointer items-center gap-1 rounded-md border border-transparent bg-transparent px-[7px] py-0.5 text-[0.66rem] text-(--theme-text-muted) transition-[background,color] duration-150 hover:bg-(--theme-surface-hover) hover:text-(--theme-text-strong)"
            onClick={() => void copy()}
            title="复制代码"
            aria-label="复制代码"
          >
            {copied ? (
              <Check className="size-3.5" />
            ) : (
              <Copy className="size-3.5" />
            )}
            <span>{copied ? '已复制' : '复制'}</span>
          </button>
        </div>
      </div>
      <pre className="m-0 max-h-120 overflow-auto rounded-none border-0 bg-transparent px-[15px] py-3">
        <code
          className="bg-transparent p-0 text-inherit"
          dangerouslySetInnerHTML={{
            __html: highlighted || '&#8203;'
          }}
        />
      </pre>
    </div>
  )
}

// 计算 ``` 围栏数量（``` 和 ```lang 都算围栏起始）。
function countCodeFences(content: string): number {
  const matches = content.match(/^```[^\n]*$/gm)
  return matches ? matches.length : 0
}

// repairStreaming 修复流式生成中的未闭合 markdown 语法，避免 markdown-to-jsx
// 把半成品当纯文本显示、完成后突然跳变（闪烁）。
//  - 奇数个代码围栏 → 补一个闭合围栏
//  - 未闭合的粗体/行内代码 → 补闭合标记（尽力而为）
function repairStreaming(content: string): string {
  let repaired = content
  if (countCodeFences(repaired) % 2 === 1) {
    repaired += '\n```'
  }
  // 未闭合行内代码（单个反引号，非围栏）
  const inlineBacktick = repaired.match(/`/g)?.length ?? 0
  if (inlineBacktick % 2 === 1) {
    repaired += '`'
  }
  // 未闭合粗体（** 或 __）
  const boldPairs = (repaired.match(/\*\*/g)?.length ?? 0) % 2
  if (boldPairs === 1) repaired += '**'
  return repaired
}

// AgentMarkdown 渲染 Agent 的 markdown 消息。streaming 表示当前消息还在流式
// 生成中：代码块显示"生成中…"，且修复未闭合的语法避免闪烁。
export function AgentMarkdown({
  content,
  streaming
}: {
  content: string
  streaming?: boolean
}) {
  const display = streaming ? repairStreaming(content) : content
  return (
    <div className="agent-markdown">
      <Markdown
        options={{
          forceBlock: true,
          overrides: {
            a: {
              component: ({ href, children, ...rest }) => (
                <a href={href} target="_blank" rel="noreferrer" {...rest}>
                  {children}
                </a>
              )
            },
            pre: {
              component: ({ children }) => (
                <CodeBlock streaming={streaming}>{children}</CodeBlock>
              )
            }
          }
        }}
      >
        {display}
      </Markdown>
    </div>
  )
}
