import { expect, test } from '@playwright/test'
import {
  parseQQInlineSegments,
  parseQQArkCard,
  qqFaceLabel,
  qqImageFailureAttempts,
  recordQQImageFailure,
  resolveQQAttachmentReferences,
  resolveQQFaceAttachments,
  type QQMessageSegment
} from '../src/components/qqMessageMedia'

const imageTag =
  '<attachmentType="image/png",attachmentIndex=0,description="eyJ0ZXh0IjoiW+WbvueJh10ifQ==">'
const builtInFaceTag =
  '<faceType=1,faceId="349",ext="eyJ0ZXh0Ijoi5Z2a5by6In0=">'
const attachedFaceTag = '<faceType=6,faceId="0",ext="eyJ0ZXh0IjoiIn0=">'

test('parses a nested QQ favorite H5 card without placeholder attachments', () => {
  const card = parseQQArkCard({
    value: {
      ark_data: {
        ark_name: '图文H5',
        ark_type: 'tuwen',
        fields: {
          title: '避雷！小红书找定制假发被骗90元',
          desc: '朋友在小红书刷到一位老师。\n[图片]\n[文件]\n结',
          jump_url: 'https://sharechain.qq.com/card',
          tag: 'QQ 收藏'
        },
        prompt: '[QQ收藏] 避雷！小红书找定制假发被骗90元'
      }
    }
  })

  expect(card).toEqual({
    arkName: '图文H5',
    arkType: 'tuwen',
    title: '避雷！小红书找定制假发被骗90元',
    description: '朋友在小红书刷到一位老师。\n结',
    prompt: '[QQ收藏] 避雷！小红书找定制假发被骗90元',
    tag: 'QQ 收藏',
    source: '',
    imageURL: '',
    sourceLogoURL: '',
    jumpURL: 'https://sharechain.qq.com/card'
  })
})

test('parses preview and source logo fields from wrapped ark data', () => {
  const card = parseQQArkCard({
    raw_message: {
      d: {
        ark_data: {
          ark_name: '小程序',
          ark_type: 'miniapp',
          fields: {
            title: '小智管家',
            source: '小智管家',
            preview: 'https://example.test/preview.png',
            source_logo: 'https://example.test/logo.png',
            jump_url: 'https://example.test/open'
          }
        }
      }
    }
  })

  expect(card).toMatchObject({
    arkName: '小程序',
    arkType: 'miniapp',
    title: '小智管家',
    source: '小智管家',
    imageURL: 'https://example.test/preview.png',
    sourceLogoURL: 'https://example.test/logo.png',
    jumpURL: 'https://example.test/open'
  })
})

test('parses an attachment protocol tag without exposing it as text', () => {
  const parsed = parseQQInlineSegments(imageTag)
  expect(parsed).toHaveLength(1)
  expect(parsed[0]).toMatchObject({
    type: 'AttachmentReference',
    value: {
      attachmentIndex: 0,
      attachmentType: 'image/png',
      description: '[图片]'
    }
  })
})

test('decodes the QQ face label carried in ext', () => {
  const parsed = parseQQInlineSegments(`收到${builtInFaceTag}`)

  expect(parsed).toHaveLength(2)
  expect(parsed[1]).toMatchObject({
    type: 'Face',
    value: {
      faceType: '1',
      faceId: '349',
      text: '坚强'
    }
  })
  expect(qqFaceLabel(parsed[1].value)).toBe('[坚强]')
})

test('binds a faceType 6 placeholder to its image attachment', () => {
  const image: QQMessageSegment = {
    type: 'ImageURL',
    value: '/api/v1/robot/chat/media?id=face',
    options: { alt: '46256904CF042A57D6065223DCDBC795.suf' }
  }
  const { segments, usedIndexes } = resolveQQFaceAttachments(
    parseQQInlineSegments(attachedFaceTag),
    [image]
  )

  expect(segments).toEqual([
    {
      type: 'QQFaceImage',
      value: image.value,
      options: {
        alt: '[QQ表情]',
        qqFace: true,
        faceType: '6',
        faceId: '0'
      }
    }
  ])
  expect([...usedIndexes]).toEqual([0])
})

test('binds multiple attachment tags to images by attachmentIndex', () => {
  const parsed = parseQQInlineSegments(
    [
      '开头',
      imageTag,
      imageTag.replace('attachmentIndex=0', 'attachmentIndex=1'),
      '结尾'
    ].join('')
  )
  const images: QQMessageSegment[] = [
    {
      type: 'ImageURL',
      value: 'https://example.test/first.png',
      options: { alt: 'first.png' }
    },
    {
      type: 'ImageURL',
      value: 'https://example.test/second.png',
      options: { alt: 'second.png' }
    }
  ]
  const { segments, usedIndexes } = resolveQQAttachmentReferences(
    parsed,
    images
  )
  expect(segments.map(segment => segment.type)).toEqual([
    'Text',
    'ImageURL',
    'ImageURL',
    'Text'
  ])
  expect(segments.map(segment => segment.value)).toEqual([
    '开头',
    images[0].value,
    images[1].value,
    '结尾'
  ])
  expect([...usedIndexes]).toEqual([0, 1])
})

test('uses a clean placeholder when an indexed image is unavailable', () => {
  const { segments } = resolveQQAttachmentReferences(
    parseQQInlineSegments(imageTag),
    []
  )
  expect(segments).toEqual([
    {
      type: 'ImageAttachment',
      options: {
        mime: 'image/png',
        alt: '图片暂不可显示'
      }
    }
  ])
})

test('does not carry a temporary URL failure into the cached URL', () => {
  const temporary = 'https://multimedia.nt.qq.com.cn/download?id=temporary'
  const cached = '/api/v1/robot/chat/media?id=cached'
  const firstFailure = recordQQImageFailure(
    { source: '', attempts: 0 },
    temporary
  )
  const secondFailure = recordQQImageFailure(firstFailure, temporary)

  expect(qqImageFailureAttempts(secondFailure, temporary)).toBe(2)
  expect(qqImageFailureAttempts(secondFailure, cached)).toBe(0)
})
