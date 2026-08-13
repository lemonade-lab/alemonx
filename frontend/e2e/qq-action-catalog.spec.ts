import { expect, test } from '@playwright/test'
import { qqActionCatalog, qqActionIDs } from '../src/components/qqActionCatalog'

// Keep this list in lockstep with @alemonjs/qq-bot/lib/register.js. The UI
// registry is intentionally tested independently so an adapter upgrade cannot
// silently hide a registered action behind the conversation window.
const registeredActions = [
  'me.info', 'message.send', 'mention.get', 'message.send.channel', 'message.send.user', 'message.send.target', 'message.delete', 'interaction.ack', 'message.pin', 'message.unpin', 'reaction.add', 'reaction.remove', 'message.get', 'interaction.response', 'member.info', 'member.list', 'member.kick', 'member.ban', 'member.unban', 'member.mute', 'guild.info', 'guild.list', 'guild.mute', 'group.info', 'group.botState', 'group.member.info', 'group.joinRequest.list', 'group.joinRequest.approve', 'group.mute.setting', 'group.mute.set', 'group.strategy.list', 'group.strategy.create', 'group.strategy.update', 'group.strategy.delete', 'group.strategy.execute', 'group.strategy.whitelist', 'channel.info', 'channel.list', 'channel.create', 'channel.update', 'channel.delete', 'role.list', 'role.create', 'role.update', 'role.delete', 'role.assign', 'role.remove', 'file.send.channel', 'file.send.user', 'me.guilds', 'media.send.channel', 'media.send.user', 'media.upload', 'media.send', 'connection.status', 'media.upload.prepare', 'media.upload.part.finish', 'media.upload.chunked', 'stream.message.send', 'message.input.notify', 'permission.get', 'permission.set', 'reaction.list', 'channel.announce'
]

test('QQ action registry covers every registered qq-bot action exactly once', () => {
  expect(registeredActions).toHaveLength(64)
  expect(qqActionIDs).toHaveLength(64)
  expect(new Set(qqActionIDs).size).toBe(64)
  expect([...qqActionIDs].sort()).toEqual([...registeredActions].sort())
  for (const action of qqActionCatalog) {
    expect(action.title).not.toBe('')
    expect(action.group).not.toBe('')
    expect(action.scopes.length).toBeGreaterThan(0)
    expect(action.fields).toBeDefined()
  }
})
