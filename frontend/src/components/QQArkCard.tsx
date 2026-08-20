import { useState } from 'react'
import { ExternalLink, FileImage, Share2 } from 'lucide-react'
import {
  qqImageFailureAttempts,
  recordQQImageFailure,
  type QQArkCard
} from './qqMessageMedia'

type QQArkCardSegmentProps = {
  card: QQArkCard
  resolveImageSource: (value: unknown) => string
}

function safeExternalURL(value: unknown) {
  if (typeof value !== 'string' || !value.trim()) return ''
  try {
    const url = new URL(value.trim())
    return url.protocol === 'http:' || url.protocol === 'https:'
      ? url.toString()
      : ''
  } catch {
    return ''
  }
}

function QQArkCardImage({
  source,
  label,
  kind
}: {
  source: string
  label: string
  kind: 'preview' | 'logo'
}) {
  const [failure, setFailure] = useState({ source: '', attempts: 0 })
  const failureAttempts = qqImageFailureAttempts(failure, source)

  if (failureAttempts < 2)
    return (
      <img
        key={`${source}:${failureAttempts}`}
        className={
          kind === 'preview'
            ? 'block size-full object-cover'
            : 'size-[22px] shrink-0 rounded-[5px] border border-(--theme-border-subtle) object-cover'
        }
        src={source}
        alt={label}
        loading="lazy"
        decoding="async"
        onLoad={() => {
          if (failureAttempts) setFailure({ source: '', attempts: 0 })
        }}
        onError={() =>
          setFailure(current => recordQQImageFailure(current, source))
        }
      />
    )

  if (kind === 'logo')
    return (
      <span className="inline-flex size-[22px] shrink-0 items-center justify-center rounded-[5px] border border-(--theme-border-subtle) bg-(--theme-surface-hover)">
        <Share2 className="size-3.5" aria-hidden="true" />
      </span>
    )

  return (
    <span className="flex flex-1 flex-col items-center justify-center gap-[5px] text-[11px] text-(--theme-text-muted)">
      <FileImage className="size-5" aria-hidden="true" />
      预览图暂不可显示
    </span>
  )
}

export function QQArkCardSegment({
  card,
  resolveImageSource
}: QQArkCardSegmentProps) {
  const jumpURL = safeExternalURL(card.jumpURL)
  const previewSource = resolveImageSource(card.imageURL)
  const sourceLogo = resolveImageSource(card.sourceLogoURL)
  const sourceLabel = card.source || card.tag || card.arkName
  const footerLabel = card.tag || card.arkName || 'QQ 卡片'
  const className = [
    'inline-flex w-[min(300px,72vw)] max-w-full flex-col overflow-hidden rounded-lg',
    'border border-(--theme-border-default) bg-(--theme-surface-panel)',
    'align-top text-left whitespace-normal text-(--theme-text-primary) no-underline',
    jumpURL
      ? 'hover:border-(--theme-accent-soft-border) hover:shadow-sm focus-visible:ring-2 focus-visible:ring-(--theme-accent)'
      : ''
  ]
    .filter(Boolean)
    .join(' ')
  const content = (
    <>
      {previewSource ? (
        <span className="flex aspect-video w-full overflow-hidden bg-(--theme-surface-hover)">
          <QQArkCardImage
            source={previewSource}
            label={`${card.title}预览图`}
            kind="preview"
          />
        </span>
      ) : null}
      <span className="flex flex-col gap-[5px] px-3 pt-[11px] pb-2.5">
        {sourceLabel || sourceLogo ? (
          <span className="flex min-w-0 items-center gap-1.5 text-[10px] text-(--theme-text-muted)">
            {sourceLogo ? (
              <QQArkCardImage
                source={sourceLogo}
                label={sourceLabel || '卡片来源'}
                kind="logo"
              />
            ) : (
              <Share2 className="size-3.5 shrink-0" aria-hidden="true" />
            )}
            {sourceLabel ? (
              <span className="truncate">{sourceLabel}</span>
            ) : null}
          </span>
        ) : null}
        <strong className="line-clamp-2 text-[13px] leading-[1.45] font-semibold text-(--theme-text-strong)">
          {card.title}
        </strong>
        {card.description ? (
          <span className="line-clamp-3 whitespace-pre-line text-[11px] leading-[1.55] text-(--theme-text-secondary)">
            {card.description}
          </span>
        ) : null}
      </span>
      <span className="flex min-h-[31px] items-center justify-between gap-2 border-t border-(--theme-border-subtle) px-3 py-1.5 text-[10px] text-(--theme-text-muted)">
        <span className="truncate">{footerLabel}</span>
        {jumpURL ? (
          <ExternalLink className="size-3.5 shrink-0" aria-hidden="true" />
        ) : null}
      </span>
    </>
  )

  if (jumpURL)
    return (
      <a
        className={className}
        href={jumpURL}
        target="_blank"
        rel="noreferrer"
        title={`打开：${card.title}`}
      >
        {content}
      </a>
    )

  return <span className={className}>{content}</span>
}
