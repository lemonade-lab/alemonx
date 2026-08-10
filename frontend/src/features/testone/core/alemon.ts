import { DataEnums, User } from '@testone/typing';
import CryptoJS from 'crypto-js';

export const Platform = 'testone';

import { User as UserType } from '@testone/typing';

/**
 * 将字符串转为定长字符串
 * @param str 输入字符串
 * @param options 可选项
 * @returns 固定长度的哈希值
 */
const createHash = (
  str: string,
  options: {
    length?: number;
    algorithm?: string;
  } = {}
) => {
  const { length = 11, algorithm = 'sha256' } = options;
  let hash: string;
  // 只支持 sha256/sha1/md5
  if (algorithm === 'sha256') {
    hash = CryptoJS.SHA256(str).toString(CryptoJS.enc.Hex);
  } else if (algorithm === 'sha1') {
    hash = CryptoJS.SHA1(str).toString(CryptoJS.enc.Hex);
  } else if (algorithm === 'md5') {
    hash = CryptoJS.MD5(str).toString(CryptoJS.enc.Hex);
  } else {
    throw new Error('只支持 sha256/sha1/md5');
  }
  return hash.slice(0, length);
};

/**
 * 使用用户的哈希键
 * @param event
 * @returns
 */
export const userHashKey = (event: { Platform: string; UserId: string }) => {
  return createHash(`${event.Platform}:${event.UserId}`);
};

export const payloadToMentions = (
  payload: {
    event: { value: DataEnums[] };
  },
  users: UserType[]
): User[] => {
  return payload.event.value
    .filter(item => item.type === 'Mention')
    .map(item => {
      const UserId = item.value;
      const user = users.find(u => u.UserId === UserId);
      if (!user) {
        return null;
      }
      const UserKey = userHashKey({
        Platform: Platform,
        UserId: UserId || ''
      });
      return {
        ...user,
        UserKey: UserKey
      };
    })
    .filter(user => !!user);
};
