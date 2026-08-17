export type QQMessageSegment = {
  type: string
  value?: unknown
  options?: Record<string, unknown>
}

export type QQImageFailure = {
  source: string
  attempts: number
}

export type QQFace = {
  faceType: string
  faceId: string
  text: string
  ext?: unknown
}

export type QQArkCard = {
  arkName: string
  arkType: string
  title: string
  description: string
  prompt: string
  tag: string
  source: string
  imageURL: string
  sourceLogoURL: string
  jumpURL: string
}

type AttachmentReference = {
  attachmentIndex: number
  attachmentType: string
  description?: unknown
}

const qqInlineSegmentPattern =
  /<(?:faceType|attachmentType)\s*=\s*(?:"[^"]*"|'[^']*'|[^,\s>]+)[^>]*>|!\[([^\]]*)\]\(([^\s)]+)(?:\s+["'][^"']*["'])?\)/g
const markdownImagePattern =
  /^!\[([^\]]*)\]\(([^\s)]+)(?:\s+["'][^"']*["'])?\)$/

export function qqImageFailureAttempts(
  failure: QQImageFailure,
  source: string
) {
  return failure.source === source ? failure.attempts : 0
}

export function recordQQImageFailure(
  failure: QQImageFailure,
  source: string
): QQImageFailure {
  return {
    source,
    attempts: failure.source === source ? failure.attempts + 1 : 1
  }
}

function recordValue(value: unknown) {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined
}

function recordText(record: Record<string, unknown>, keys: string[]) {
  for (const key of keys) {
    const value = record[key]
    if (typeof value === 'string' && value.trim()) return value.trim()
    if (typeof value === 'number' && Number.isFinite(value))
      return String(value)
  }
  return ''
}

export function qqFaceData(value: unknown): QQFace {
  const record = recordValue(value) || {}
  return {
    faceType: recordText(record, ['faceType', 'face_type', 'type']),
    faceId: recordText(record, ['faceId', 'face_id', 'id']),
    text: recordText(record, ['text', 'name', 'label']),
    ext: record.ext
  }
}

export function qqFaceLabel(value: unknown) {
  const face = qqFaceData(value)
  if (face.text)
    return face.text.startsWith('[') && face.text.endsWith(']')
      ? face.text
      : `[${face.text}]`
  if (face.faceType === '6') return '[QQ表情]'
  return face.faceId ? `[QQ表情 ${face.faceId}]` : '[QQ表情]'
}

function cleanCardDescription(value: string) {
  return value
    .replace(/(^|\n)\s*\[(?:图片|文件|视频|语音|附件)\]\s*(?=\n|$)/g, '$1')
    .replace(/\n{2,}/g, '\n')
    .trim()
}

function findArkData(value: unknown) {
  const visited = new Set<object>()
  const visit = (node: unknown): Record<string, unknown> | undefined => {
    if (!node || typeof node !== 'object' || visited.has(node)) return undefined
    visited.add(node)
    if (Array.isArray(node)) {
      for (const item of node) {
        const match = visit(item)
        if (match) return match
      }
      return undefined
    }
    const record = node as Record<string, unknown>
    const direct = recordValue(record.ark_data)
    if (direct) return direct
    for (const key of [
      'value',
      'data',
      'payload',
      'raw_message',
      'raw',
      'd',
      'message',
      'Message'
    ]) {
      const match = visit(record[key])
      if (match) return match
    }
    return undefined
  }
  return visit(value)
}

export function parseQQArkCard(value: unknown): QQArkCard | undefined {
  const ark = findArkData(value)
  if (!ark) return undefined
  const fields = recordValue(ark.fields) || {}
  const arkName = recordText(ark, ['ark_name', 'arkName', 'name'])
  const arkType = recordText(ark, ['ark_type', 'arkType', 'type'])
  const prompt = recordText(ark, ['prompt', 'summary'])
  const title =
    recordText(fields, ['title', 'name', 'nickname']) ||
    recordText(ark, ['title']) ||
    prompt ||
    arkName ||
    '卡片消息'
  const description = cleanCardDescription(
    recordText(fields, ['desc', 'description', 'address']) ||
      recordText(ark, ['desc', 'description'])
  )
  return {
    arkName,
    arkType,
    title,
    description,
    prompt,
    tag: recordText(fields, ['tag']) || recordText(ark, ['tag']) || arkName,
    source: recordText(fields, ['source', 'app_name', 'appName']),
    imageURL:
      recordText(fields, [
        'preview',
        'image',
        'image_url',
        'imageUrl',
        'cover',
        'cover_url'
      ]) || recordText(ark, ['image', 'image_url', 'preview']),
    sourceLogoURL:
      recordText(fields, [
        'source_logo',
        'sourceLogo',
        'source_logo_url',
        'icon',
        'icon_url'
      ]) || recordText(ark, ['source_logo', 'icon']),
    jumpURL:
      recordText(fields, ['jump_url', 'jumpUrl', 'url']) ||
      recordText(ark, ['jump_url', 'jumpUrl', 'url'])
  }
}

function tagAttributes(tag: string) {
  const attributes: Record<string, string> = {}
  const matcher = /([A-Za-z_][\w-]*)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^,\s>]+))/g
  for (const match of tag.matchAll(matcher))
    attributes[match[1]] = match[2] ?? match[3] ?? match[4] ?? ''
  return attributes
}

function decodeTagPayload(value?: string) {
  if (!value) return undefined
  try {
    const normalized = value.replace(/-/g, '+').replace(/_/g, '/')
    const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=')
    const bytes = Uint8Array.from(atob(padded), item => item.charCodeAt(0))
    return JSON.parse(new TextDecoder().decode(bytes)) as Record<
      string,
      unknown
    >
  } catch {
    return undefined
  }
}

function parseFaceTag(tag: string): QQMessageSegment {
  const attributes = tagAttributes(tag)
  const ext = decodeTagPayload(attributes.ext)
  return {
    type: 'Face',
    value: {
      faceType: attributes.faceType || '',
      faceId: attributes.faceId || '',
      text: ext ? recordText(ext, ['text', 'name', 'label']) : '',
      ext: ext ?? attributes.ext
    } satisfies QQFace
  }
}

function parseAttachmentTag(tag: string): QQMessageSegment {
  const attributes = tagAttributes(tag)
  const decoded = decodeTagPayload(attributes.description)
  const description = decoded
    ? recordText(decoded, ['text', 'name', 'label']) || decoded
    : attributes.description
  const parsedIndex = Number.parseInt(attributes.attachmentIndex || '', 10)
  return {
    type: 'AttachmentReference',
    value: {
      attachmentIndex: Number.isInteger(parsedIndex) ? parsedIndex : -1,
      attachmentType: attributes.attachmentType || '',
      description
    } satisfies AttachmentReference,
    options: {
      mime: attributes.attachmentType || '',
      alt: attributes.attachmentType.startsWith('image/') ? '图片' : '附件'
    }
  }
}

function parseMarkdownImage(text: string): QQMessageSegment | undefined {
  const match = text.match(markdownImagePattern)
  if (!match) return undefined
  return {
    type: 'ImageURL',
    value: match[2],
    options: { alt: match[1] || '图片' }
  }
}

export function parseQQInlineSegments(text: string) {
  const segments: QQMessageSegment[] = []
  let cursor = 0
  for (const match of text.matchAll(qqInlineSegmentPattern)) {
    const index = match.index ?? 0
    if (index > cursor)
      segments.push({ type: 'Text', value: text.slice(cursor, index) })
    const token = match[0]
    segments.push(
      parseMarkdownImage(token) ||
        (token.startsWith('<attachmentType')
          ? parseAttachmentTag(token)
          : parseFaceTag(token))
    )
    cursor = index + token.length
  }
  if (cursor < text.length)
    segments.push({ type: 'Text', value: text.slice(cursor) })
  return segments.length ? segments : [{ type: 'Text', value: text }]
}

export function resolveQQAttachmentReferences(
  segments: QQMessageSegment[],
  indexedImages: Array<QQMessageSegment | undefined>
) {
  const usedIndexes = new Set<number>()
  const resolved = segments.map(segment => {
    if (segment.type !== 'AttachmentReference') return segment
    const reference = recordValue(segment.value) as
      AttachmentReference | undefined
    const index = reference?.attachmentIndex ?? -1
    const mime = reference?.attachmentType || ''
    const image = index >= 0 ? indexedImages[index] : undefined
    if (image) {
      usedIndexes.add(index)
      return {
        ...image,
        options: {
          ...(segment.options || {}),
          ...(image.options || {}),
          mime: mime || image.options?.mime
        }
      }
    }
    const isImage = mime.startsWith('image/')
    return {
      type: isImage ? 'ImageAttachment' : 'FileAttachment',
      options: {
        ...(segment.options || {}),
        mime,
        alt: isImage ? '图片暂不可显示' : '附件暂不可显示'
      }
    }
  })
  return { segments: resolved, usedIndexes }
}

export function resolveQQFaceAttachments(
  segments: QQMessageSegment[],
  indexedImages: Array<QQMessageSegment | undefined>,
  reservedIndexes: ReadonlySet<number> = new Set()
) {
  const usedIndexes = new Set(reservedIndexes)
  let imageCursor = 0
  const nextImage = () => {
    while (imageCursor < indexedImages.length) {
      const index = imageCursor
      imageCursor += 1
      if (usedIndexes.has(index)) continue
      const image = indexedImages[index]
      if (!image) continue
      usedIndexes.add(index)
      return image
    }
    return undefined
  }
  const resolved = segments.map(segment => {
    if (segment.type !== 'Face') return segment
    const face = qqFaceData(segment.value)
    if (face.faceType !== '6') return segment
    const image = nextImage()
    if (!image) return segment
    return {
      ...image,
      type: 'QQFaceImage',
      options: {
        ...(image.options || {}),
        qqFace: true,
        faceType: face.faceType,
        faceId: face.faceId,
        alt: qqFaceLabel(face)
      }
    }
  })
  return { segments: resolved, usedIndexes }
}
