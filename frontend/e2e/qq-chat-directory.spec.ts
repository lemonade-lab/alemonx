import { expect, test } from '@playwright/test'
import {
  extractQQGroupIdentity,
  findIdentityRecord,
  qqConversationAvatarSource,
  qqGroupAvatarSource,
  recordText
} from '../src/components/qqChatDirectory'

const expected = {
  name: '测试群',
  groupNumber: '123456789',
  avatar: 'https://p.qlogo.cn/gh/123456789/123456789/100'
}

for (const [title, payload] of [
  ['data array', { data: [{ group_name: '测试群', group_num: '123456789' }] }],
  [
    'nested group_info',
    {
      result: {
        group_info: { group_name: '测试群', group_num: '123456789' }
      }
    }
  ],
  [
    'complete action results',
    [
      {
        code: 2000,
        data: { group_name: '测试群', group_num: '123456789' }
      }
    ]
  ],
  [
    'JSON string body',
    JSON.stringify({
      data: { group_name: '测试群', group_num: '123456789' }
    })
  ]
] as const) {
  test(`extracts QQ group identity from ${title}`, () => {
    expect(extractQQGroupIdentity(payload)).toEqual(expected)
  })
}

test('findIdentityRecord ignores action envelopes and reads numeric group numbers', () => {
  const record = findIdentityRecord(
    {
      code: 2000,
      message: 'success',
      payload: {
        response: {
          groupInfo: { groupName: '数字群号', groupNum: 987654321 }
        }
      }
    },
    ['group_name', 'group_num']
  )
  expect(record).toBeDefined()
  expect(recordText(record || {}, ['group_name'])).toBe('数字群号')
  expect(recordText(record || {}, ['group_num'])).toBe('987654321')
})

test('group conversations never fall back to the last speaker avatar', () => {
  const speakerAvatar = 'https://q.qlogo.cn/qqapp/102000000/USER_OPENID/640'
  expect(qqConversationAvatarSource('group', '', '', speakerAvatar)).toBe('')
  expect(qqGroupAvatarSource(speakerAvatar)).toBe('')
})

test('group conversations reject member avatars from other image hosts', () => {
  const speakerAvatar = 'https://third-party.example/avatar/member.jpg'
  expect(
    qqConversationAvatarSource('group', speakerAvatar, speakerAvatar, '')
  ).toBe('')
  expect(qqGroupAvatarSource(speakerAvatar)).toBe('')
})

test('generic avatar fields in group info are not treated as group avatars', () => {
  expect(
    extractQQGroupIdentity({
      data: {
        group_name: '测试群',
        avatar: 'https://third-party.example/avatar/member.jpg'
      }
    })
  ).toEqual({
    name: '测试群',
    groupNumber: '',
    avatar: ''
  })
})

test('group conversations keep a real group avatar', () => {
  const groupAvatar = expected.avatar
  expect(
    qqConversationAvatarSource(
      'group',
      groupAvatar,
      '',
      'https://q.qlogo.cn/qqapp/102000000/USER_OPENID/640'
    )
  ).toBe(groupAvatar)
})

test('private conversations can use the sender avatar', () => {
  const speakerAvatar = 'https://q.qlogo.cn/qqapp/102000000/USER_OPENID/640'
  expect(qqConversationAvatarSource('c2c', '', '', speakerAvatar)).toBe(
    speakerAvatar
  )
})
