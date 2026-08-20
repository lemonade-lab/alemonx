export type QQScope = 'group' | 'channel' | 'c2c' | 'direct' | 'global'
export type FieldKind = 'text' | 'number' | 'textarea' | 'select' | 'boolean' | 'csv' | 'file' | 'url'

export type QQActionField = {
  key: string
  label: string
  kind?: FieldKind
  required?: boolean
  placeholder?: string
  options?: Array<[string, string]>
  help?: string
}

export type QQActionDefinition = {
  id: string
  title: string
  group: string
  scopes: QQScope[]
  fields: QQActionField[]
  risk?: 'high'
  description?: string
}

const text = (key: string, label: string, required = false, placeholder?: string): QQActionField => ({ key, label, required, placeholder })
const number = (key: string, label: string, required = false, placeholder?: string): QQActionField => ({ key, label, kind: 'number', required, placeholder })
const textarea = (key: string, label: string, required = false, placeholder?: string): QQActionField => ({ key, label, kind: 'textarea', required, placeholder })
const csv = (key: string, label: string, required = false, placeholder?: string): QQActionField => ({ key, label, kind: 'csv', required, placeholder })
const bool = (key: string, label: string, help?: string): QQActionField => ({ key, label, kind: 'boolean', help })
const choice = (key: string, label: string, options: Array<[string, string]>, required = false): QQActionField => ({ key, label, kind: 'select', options, required })
const file = (key: string, label: string, required = false): QQActionField => ({ key, label, kind: 'file', required })
const url = (key: string, label: string, required = false): QQActionField => ({ key, label, kind: 'url', required, placeholder: 'https://…' })

const officialSendFields = [
  textarea('formatText', '文本内容', false),
  textarea('markdown', 'Markdown 内容', false, '留空时按普通文本发送；填写后使用 msg_type=2。'),
  textarea('keyboard', '按钮 Keyboard JSON', false, '{"content":{"rows":[...]}}'),
  text('replyId', '回复的消息 ID'),
  number('msg_seq', '消息序号'),
  choice('msg_type', '消息类型', [['0', '文本'], ['2', 'Markdown'], ['7', '富媒体']])
]

// This registry is deliberately the sole source for the tool centre. It is
// kept one-for-one with @alemonjs/qq-bot's registered onAction branches.
// Forms use scalar and list controls only; no tool needs callers to compose a
// raw JSON payload. Context such as event, target, guild/channel/member ids is
// added by LiveChat immediately before an action is sent.
export const qqActionCatalog: QQActionDefinition[] = [
  { id: 'me.info', title: '机器人资料', group: '机器人状态', scopes: ['global'], fields: [] },
  { id: 'me.guilds', title: '机器人频道列表', group: '机器人状态', scopes: ['global'], fields: [] },
  { id: 'connection.status', title: '连接状态', group: '机器人状态', scopes: ['global'], fields: [] },
  { id: 'guild.list', title: '频道列表', group: '机器人状态', scopes: ['global'], fields: [] },

  { id: 'message.send', title: '按当前会话发送', group: '消息与互动', scopes: ['group', 'channel', 'c2c', 'direct'], fields: officialSendFields, description: '普通输入框发送时会使用此动作；工具表单可发送 Markdown、按钮和回复。' },
  { id: 'message.send.channel', title: '向群/频道发送', group: '消息与互动', scopes: ['group', 'channel'], fields: [text('ChannelId', '目标群或频道 ID', true), ...officialSendFields] },
  { id: 'message.send.user', title: '向用户发送', group: '消息与互动', scopes: ['c2c', 'direct'], fields: [text('UserId', '目标用户 ID', true), ...officialSendFields] },
  { id: 'message.send.target', title: '向指定目标发送', group: '消息与互动', scopes: ['group', 'channel', 'c2c', 'direct'], fields: [text('targetId', '目标 ID', true), choice('targetScope', '目标类型', [['group', '群'], ['c2c', '私聊'], ['channel', '子频道'], ['direct', '频道私信']], true), ...officialSendFields] },
  { id: 'message.delete', title: '撤回消息', group: '消息与互动', scopes: ['group', 'channel', 'c2c', 'direct'], fields: [text('MessageId', '消息 ID', true)], risk: 'high' },
  { id: 'message.get', title: '读取频道消息', group: '消息与互动', scopes: ['channel'], fields: [text('MessageId', '消息 ID', true)] },
  { id: 'message.pin', title: '设为精华', group: '消息与互动', scopes: ['channel'], fields: [text('MessageId', '消息 ID', true)] },
  { id: 'message.unpin', title: '取消精华', group: '消息与互动', scopes: ['channel'], fields: [text('MessageId', '消息 ID', true)] },
  { id: 'message.input.notify', title: '输入状态', group: '消息与互动', scopes: ['c2c'], fields: [choice('input_type', '状态', [['1', '正在输入']], true), number('input_second', '展示秒数', false, '60'), number('msg_seq', '消息序号'), text('msg_id', '关联消息 ID')] },
  { id: 'mention.get', title: '读取 @ 提及', group: '消息与互动', scopes: ['group', 'channel', 'c2c', 'direct'], fields: [] },
  { id: 'interaction.ack', title: '确认交互', group: '消息与互动', scopes: ['group', 'channel', 'c2c', 'direct'], fields: [text('interactionId', '交互 ID', true), choice('code', '处理结果', [['0', '成功'], ['1', '失败']], true)] },
  { id: 'interaction.response', title: '群交互响应', group: '消息与互动', scopes: ['group'], fields: [text('interaction_id', '交互 ID', true), choice('code', '处理结果', [['0', '成功'], ['1', '失败']], true)] },
  { id: 'reaction.add', title: '添加表态', group: '消息与互动', scopes: ['channel'], fields: [text('MessageId', '消息 ID', true), text('EmojiId', '表态 Emoji ID', true)] },
  { id: 'reaction.remove', title: '移除表态', group: '消息与互动', scopes: ['channel'], fields: [text('MessageId', '消息 ID', true), text('EmojiId', '表态 Emoji ID', true)] },
  { id: 'reaction.list', title: '查看表态用户', group: '消息与互动', scopes: ['channel'], fields: [text('MessageId', '消息 ID', true), text('EmojiId', '表态 Emoji ID', true), number('limit', '每页数量', false, '20')] },

  { id: 'file.send.channel', title: '发送群文件', group: '媒体与文件', scopes: ['group'], fields: [choice('file_type', '文件类型', [['1', '普通文件'], ['2', '图片'], ['3', '视频'], ['4', '音频']], true), file('file_path', '本地文件'), url('url', '文件 URL'), bool('srv_send_msg', '同时发送一条消息')] },
  { id: 'file.send.user', title: '发送私聊文件', group: '媒体与文件', scopes: ['c2c'], fields: [choice('file_type', '文件类型', [['1', '普通文件'], ['2', '图片'], ['3', '视频'], ['4', '音频']], true), file('file_path', '本地文件'), url('url', '文件 URL'), bool('srv_send_msg', '同时发送一条消息')] },
  { id: 'media.send', title: '上传并发送媒体', group: '媒体与文件', scopes: ['group', 'c2c'], fields: [choice('type', '媒体类型', [['image', '图片'], ['video', '视频'], ['audio', '音频'], ['file', '文件']], true), file('filePath', '本地媒体'), url('url', '媒体 URL'), text('content', '附带文本')] },
  { id: 'media.upload', title: '上传媒体', group: '媒体与文件', scopes: ['group', 'c2c'], fields: [choice('type', '媒体类型', [['image', '图片'], ['video', '视频'], ['audio', '音频'], ['file', '文件']], true), file('filePath', '本地媒体'), url('url', '媒体 URL')] },
  { id: 'media.send.user', title: '向用户发送富媒体', group: '媒体与文件', scopes: ['c2c'], fields: [choice('type', '媒体类型', [['image', '图片'], ['video', '视频'], ['audio', '音频'], ['file', '文件']], true), file('filePath', '本地媒体'), url('url', '媒体 URL'), text('data', '媒体数据') ] },
  { id: 'media.send.channel', title: '向频道发送富媒体', group: '媒体与文件', scopes: ['channel'], fields: [choice('type', '媒体类型', [['image', '图片'], ['video', '视频'], ['audio', '音频'], ['file', '文件']], true), url('url', '媒体 URL')], description: '当前 qq-bot 适配器会返回兼容性提示；请用“按当前会话发送”发送频道媒体。' },
  { id: 'media.upload.prepare', title: '准备分片上传', group: '媒体与文件', scopes: ['group', 'c2c'], fields: [text('file_name', '文件名', true), number('file_size', '文件大小（字节）', true), choice('file_type', '文件类型', [['1', '普通文件'], ['2', '图片'], ['3', '视频'], ['4', '音频']], true), text('md5', '文件 MD5', true), text('sha1', '文件 SHA1', true), text('md5_10m', '前 10 MiB 的 MD5', true)] },
  { id: 'media.upload.part.finish', title: '完成分片上传', group: '媒体与文件', scopes: ['group', 'c2c'], fields: [text('upload_id', '上传 ID', true), number('part_index', '分片序号', true), number('block_size', '分片字节数', true), text('md5', '该分片 MD5', true)] },
  { id: 'media.upload.chunked', title: '分片上传文件', group: '媒体与文件', scopes: ['group', 'c2c'], fields: [file('file_path', '本地文件', true), choice('file_type', '文件类型', [['1', '普通文件'], ['2', '图片'], ['3', '视频'], ['4', '音频']], true), text('file_name', '显示文件名'), bool('srv_send_msg', '上传后发送消息')] },
  { id: 'stream.message.send', title: '流式消息', group: '媒体与文件', scopes: ['c2c'], fields: [text('msg_id', '关联消息 ID'), number('msg_seq', '消息序号'), text('content_raw', '当前分片内容', true), choice('input_mode', '更新方式', [['append', '追加'], ['replace', '替换']], true), choice('content_type', '内容类型', [['text', '纯文本'], ['markdown', 'Markdown']], true), text('stream_msg_id', '流消息 ID'), number('index', '分片序号'), bool('is_wakeup', '唤醒用户')] },

  { id: 'group.info', title: '群信息', group: '群管理', scopes: ['group'], fields: [] },
  { id: 'group.botState', title: '机器人群状态', group: '群管理', scopes: ['group'], fields: [] },
  { id: 'group.member.info', title: '群成员资料', group: '群管理', scopes: ['group'], fields: [text('memberOpenId', '成员 OpenID', true)] },
  { id: 'group.joinRequest.list', title: '入群申请列表', group: '入群策略', scopes: ['group'], fields: [text('cursor', '分页游标'), number('limit', '每页数量', false, '20')] },
  { id: 'group.joinRequest.approve', title: '处理入群申请', group: '入群策略', scopes: ['group'], fields: [text('memberOpenId', '申请成员 OpenID', true), text('joinRequestId', '申请 ID', true), choice('op', '处理决定', [['approve', '同意'], ['decline', '拒绝']], true), text('rejectReason', '拒绝原因'), bool('addToMemberBlacklist', '加入黑名单')], risk: 'high' },
  { id: 'group.mute.setting', title: '群禁言设置', group: '群管理', scopes: ['group'], fields: [] },
  { id: 'group.mute.set', title: '设置群禁言', group: '群管理', scopes: ['group'], fields: [{ key: 'members', label: '成员禁言规则', kind: 'textarea', required: true, placeholder: '每行：操作(add/update/del), 成员 OpenID, 到期 Unix 时间戳；例如 add, user_openid, 1760000000' }], risk: 'high' },
  { id: 'group.strategy.list', title: '入群策略列表', group: '入群策略', scopes: ['group', 'global'], fields: [text('cursor', '分页游标'), number('limit', '每页数量', false, '20')] },
  { id: 'group.strategy.create', title: '创建入群策略', group: '入群策略', scopes: ['group', 'global'], fields: [csv('groupOpenIds', '群 OpenID 列表'), csv('groupIds', '群 ID 列表'), choice('isEnable', '策略状态', [['on', '启用'], ['off', '停用']], true), number('expireAt', '失效时间戳'), text('remark', '备注')] },
  { id: 'group.strategy.update', title: '更新入群策略', group: '入群策略', scopes: ['group', 'global'], fields: [text('StrategyId', '策略 ID', true), choice('isEnable', '策略状态', [['on', '启用'], ['off', '停用']]), choice('groupActionOp', '群名单操作', [['add', '添加群'], ['del', '移除群']]), csv('groupActionOpenIds', '操作群 OpenID'), csv('groupActionIds', '操作群 ID'), number('expireAt', '失效时间戳'), text('remark', '备注')], risk: 'high' },
  { id: 'group.strategy.delete', title: '删除入群策略', group: '入群策略', scopes: ['group', 'global'], fields: [text('StrategyId', '策略 ID', true)], risk: 'high' },
  { id: 'group.strategy.execute', title: '执行入群策略', group: '入群策略', scopes: ['group', 'global'], fields: [text('StrategyId', '策略 ID', true)], risk: 'high' },
  { id: 'group.strategy.whitelist', title: '管理策略白名单', group: '入群策略', scopes: ['group', 'global'], fields: [text('StrategyId', '策略 ID', true), choice('op', '白名单操作', [['add', '添加'], ['del', '移除']], true), csv('whitelistUsers', '用户 OpenID 列表', true)], risk: 'high' },

  { id: 'guild.info', title: '频道信息', group: '频道管理', scopes: ['channel', 'direct'], fields: [] },
  { id: 'guild.mute', title: '频道全员禁言', group: '频道管理', scopes: ['channel', 'direct'], fields: [number('duration', '禁言秒数', true, '0 表示解除')], risk: 'high' },
  { id: 'channel.info', title: '子频道信息', group: '频道管理', scopes: ['channel'], fields: [] },
  { id: 'channel.list', title: '子频道列表', group: '频道管理', scopes: ['channel', 'direct'], fields: [] },
  { id: 'channel.create', title: '创建子频道', group: '频道管理', scopes: ['channel', 'direct'], fields: [text('name', '名称', true), choice('type', '类型', [['0', '文字'], ['2', '语音'], ['4', '分类']], true), text('parentId', '父频道 ID')] },
  { id: 'channel.update', title: '更新子频道', group: '频道管理', scopes: ['channel'], fields: [text('name', '名称'), number('position', '排序位置')] },
  { id: 'channel.delete', title: '删除子频道', group: '频道管理', scopes: ['channel'], fields: [], risk: 'high' },
  { id: 'channel.announce', title: '频道公告', group: '频道管理', scopes: ['channel', 'direct'], fields: [text('messageId', '消息 ID', true), text('channelId', '公告频道 ID'), bool('remove', '删除公告')], risk: 'high' },

  { id: 'member.info', title: '频道成员资料', group: '成员', scopes: ['channel', 'direct'], fields: [text('userId', '成员用户 ID')] },
  { id: 'member.list', title: '频道成员列表', group: '成员', scopes: ['channel', 'direct'], fields: [text('After', '分页游标', false, '0'), number('Limit', '每页数量', false, '100')] },
  { id: 'member.kick', title: '移出频道成员', group: '成员', scopes: ['channel', 'direct'], fields: [text('UserId', '成员用户 ID', true)], risk: 'high' },
  { id: 'member.ban', title: '禁言频道成员', group: '成员', scopes: ['channel', 'direct'], fields: [text('UserId', '成员用户 ID', true), number('duration', '禁言秒数', true, '604800')], risk: 'high' },
  { id: 'member.unban', title: '解除成员禁言', group: '成员', scopes: ['channel', 'direct'], fields: [text('UserId', '成员用户 ID', true)], risk: 'high' },
  { id: 'member.mute', title: '设置成员禁言', group: '成员', scopes: ['channel', 'direct'], fields: [text('UserId', '成员用户 ID', true), number('duration', '禁言秒数', true)], risk: 'high' },

  { id: 'role.list', title: '身份组列表', group: '身份组', scopes: ['channel', 'direct'], fields: [] },
  { id: 'role.create', title: '创建身份组', group: '身份组', scopes: ['channel', 'direct'], fields: [text('name', '名称', true), number('color', '颜色值')], risk: 'high' },
  { id: 'role.update', title: '更新身份组', group: '身份组', scopes: ['channel', 'direct'], fields: [text('RoleId', '身份组 ID', true), text('name', '名称', true), number('color', '颜色值')], risk: 'high' },
  { id: 'role.delete', title: '删除身份组', group: '身份组', scopes: ['channel', 'direct'], fields: [text('RoleId', '身份组 ID', true)], risk: 'high' },
  { id: 'role.assign', title: '分配身份组', group: '身份组', scopes: ['channel', 'direct'], fields: [text('RoleId', '身份组 ID', true), text('UserId', '成员用户 ID', true)], risk: 'high' },
  { id: 'role.remove', title: '移除身份组', group: '身份组', scopes: ['channel', 'direct'], fields: [text('RoleId', '身份组 ID', true), text('UserId', '成员用户 ID', true)], risk: 'high' },

  { id: 'permission.get', title: '读取频道权限', group: '权限', scopes: ['channel'], fields: [text('UserId', '用户 ID', true)] },
  { id: 'permission.set', title: '设置频道权限', group: '权限', scopes: ['channel'], fields: [text('UserId', '用户 ID', true), text('allow', '允许权限位', true, '0'), text('deny', '拒绝权限位', true, '0')], risk: 'high' }
]

export const qqActionIDs = qqActionCatalog.map(item => item.id)

export function isActionAvailable(action: QQActionDefinition, scope: QQScope | '') {
  return action.scopes.includes('global') || (!!scope && action.scopes.includes(scope))
}
