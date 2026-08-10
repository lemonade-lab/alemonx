// AlemonJS 消息数据段（DataEnums）的本地等价类型。testone 只按结构使用
// type/value/options，不依赖 alemonjs 运行时包。
export type DataEnums = {
  type: string
  value?: any
  options?: any
  [key: string]: any
};

export type Channel = {
  GuildId: string;
  ChannelId: string;
  ChannelAvatar: string;
  ChannelName: string;
};

export type Channels = Channel[];

export type Connect = {
  host: string;
  port: number;
  name: string;
};

export type User = {
  UserId: string;
  UserName: string;
  UserAvatar: string;
  OpenId?: string;
  IsMaster: boolean;
  IsBot: boolean;
};

export const Users: User[] = [];

export type PageTab = 'group' | 'private';

export type Reaction = {
  emoji: string;
  users: string[]; // UserIds
};

export type MessageItem = {
  UserId: string;
  UserName: string;
  UserAvatar: string;
  CreateAt: number;
  MessageId?: string;
  data: DataEnums[];
  reactions?: Reaction[];
  UpdateAt?: number; // 编辑时间
  IsEdited?: boolean; // 是否被编辑过
};

export type SystemNotification = {
  id: string;
  type: 'notice' | 'member_change' | 'channel_change' | 'guild_change';
  CreateAt: number;
  title: string;
  content: string;
  data?: any;
};

export type MemberChangeEvent = {
  type: 'add' | 'remove' | 'ban' | 'unban' | 'update';
  UserId: string;
  UserName?: string;
  GuildId?: string;
  Reason?: string;
};

export type Command = {
  title: string;
  description: string;
  text: string;
  autoEnter?: boolean;
  data?: DataEnums[];
};
export type Commands = Command[];
