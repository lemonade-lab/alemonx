import { DataEnums } from '@testone/typing';
import { parseMessage } from './parse';

interface User {
  UserId: string;
  UserName: string;
  UserAvatar: string;
}
interface Channel {
  ChannelId: string;
  ChannelName: string;
  ChannelAvatar: string;
}

let worker: Worker | null = null;
let seq = 0;
const pending = new Map<
  number,
  { resolve: (v: DataEnums[]) => void; reject: (e: any) => void; timer: any }
>();

function ensureWorker() {
  if (worker) {
    return worker;
  }
  try {
    worker = new Worker(new URL('../workers/parseWorker.ts', import.meta.url), {
      type: 'module'
    });
    worker.onmessage = (e: MessageEvent) => {
      const { id, ok, data, error } = e.data || {};
      const item = pending.get(id);
      if (!item) {
        return;
      }
      pending.delete(id);
      clearTimeout(item.timer);
      if (ok) {
        item.resolve(data as DataEnums[]);
      } else {
        item.reject(new Error(error));
      }
    };
    worker.onerror = () => {
      try {
        worker?.terminate();
      } catch {}
      worker = null;
    };
  } catch {
    worker = null;
  }
  return worker;
}

export function parseMessageSmart({
  input,
  users,
  channels
}: {
  input: string;
  users: User[];
  channels: Channel[];
}): Promise<DataEnums[]> {
  if (input.length < 180 || typeof window === 'undefined') {
    return Promise.resolve(
      parseMessage({ input, Users: users as any, Channels: channels as any })
    );
  }
  const w = ensureWorker();
  if (!w) {
    return Promise.resolve(
      parseMessage({ input, Users: users as any, Channels: channels as any })
    );
  }
  return new Promise((resolve, reject) => {
    const id = ++seq;
    const timer = setTimeout(() => {
      pending.delete(id);
      try {
        w.terminate();
      } catch {}
      worker = null;
      resolve(
        parseMessage({ input, Users: users as any, Channels: channels as any })
      );
    }, 1500);
    pending.set(id, { resolve, reject, timer });
    w.postMessage({ id, input, users, channels });
  });
}
