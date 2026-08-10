// 图片引用存储与规范化工具
// 目的：将大体积 base64 图片内容转为引用以减少聊天主数组内存和裁剪频率

import { LOCAL_STORAGE_IMAGE_COMPRESSION } from './config';

export interface ImageRefValue {
  hash: string; // 内容哈希
  size: number; // 原/压缩后 base64 长度
}
const PREFIX = 'img:'; // 旧 localStorage 前缀（仍做兼容读取）
const TARGET_MAX_STORED = 220 * 1024; // 压缩后目标最大体积
const MAX_DIMENSION = 800; // 限制最大宽高，防止巨图占用
const MAX_BLOB_COUNT = 120; // 内存最多缓存多少张

interface BlobEntry {
  url: string;
  size: number;
  ts: number;
}
const _blobMap = new Map<string, BlobEntry>();

function ensureBlobBudget() {
  if (_blobMap.size <= MAX_BLOB_COUNT) {
    return;
  }
  const entries = Array.from(_blobMap.entries()).sort(
    (a, b) => a[1].ts - b[1].ts
  );
  while (entries.length && _blobMap.size > Math.floor(MAX_BLOB_COUNT * 0.9)) {
    const [hash, ent] = entries.shift()!;
    URL.revokeObjectURL(ent.url);
    _blobMap.delete(hash);
  }
}

export function getImageObjectUrl(hash: string): string | undefined {
  const ent = _blobMap.get(hash);
  if (ent) {
    ent.ts = Date.now();
    return ent.url;
  }
  // 兼容：若旧版本遗留 localStorage base64，可懒加载进 Blob
  try {
    const legacy = localStorage.getItem(PREFIX + hash);
    if (legacy) {
      const blob = base64ToBlob(legacy, 'image/png');
      const url = URL.createObjectURL(blob);
      _blobMap.set(hash, { url, size: legacy.length, ts: Date.now() });
      ensureBlobBudget();
      return url;
    }
  } catch {}
  return undefined;
}

function djb2(str: string) {
  let h = 5381;
  for (let i = 0; i < str.length; i++) {
    h = (h * 33) ^ str.charCodeAt(i);
  }
  return (h >>> 0).toString(16);
}

export function extractInlineBase64(v: string): string | null {
  if (!v) {
    return null;
  }
  if (v.startsWith('base64://')) {
    return v.slice(9);
  }
  if (/^data:image\//.test(v)) {
    const i = v.indexOf('base64,');
    if (i >= 0) {
      return v.slice(i + 7);
    }
  }
  if (/^[A-Za-z0-9+/=]+$/.test(v) && v.length > 100) {
    return v;
  } // 粗略判断
  return null;
}

export function storeBase64(b64: string): ImageRefValue | null {
  if (!b64) {
    return null;
  }
  // if (b64.length > MAX_BASE64_LEN) {
  //   b64 = b64.slice(0, MAX_BASE64_LEN);
  // }
  const hash = djb2(b64);
  if (!_blobMap.has(hash)) {
    try {
      const blob = base64ToBlob(b64, 'image/jpeg');
      const url = URL.createObjectURL(blob);
      _blobMap.set(hash, { url, size: b64.length, ts: Date.now() });
      ensureBlobBudget();
    } catch {
      return null;
    }
  } else {
    // 触摸更新时间
    const ent = _blobMap.get(hash)!;
    ent.ts = Date.now();
  }
  return { hash, size: b64.length };
}

async function compressBase64IfNeeded(rawB64: string): Promise<string> {
  try {
    if (typeof document === 'undefined') {
      return rawB64;
    } // 非浏览器环境跳过
    if (rawB64.length <= TARGET_MAX_STORED) {
      return rawB64;
    } // 已够小
    const dataUrl = `data:image/png;base64,${rawB64}`;
    const img = new Image();
    const loaded: HTMLImageElement = await new Promise((resolve, reject) => {
      img.onload = () => resolve(img);
      img.onerror = e => reject(e);
      img.src = dataUrl;
    });
    let { width, height } = loaded;
    if (width <= 0 || height <= 0) {
      return rawB64;
    }
    let scale = 1;
    if (width > MAX_DIMENSION || height > MAX_DIMENSION) {
      scale = Math.min(MAX_DIMENSION / width, MAX_DIMENSION / height);
    }
    const canvas = document.createElement('canvas');
    canvas.width = Math.max(1, Math.floor(width * scale));
    canvas.height = Math.max(1, Math.floor(height * scale));
    const ctx = canvas.getContext('2d');
    if (!ctx) {
      return rawB64;
    }
    ctx.drawImage(loaded, 0, 0, canvas.width, canvas.height);
    let quality = 0.82;
    let best = rawB64;
    for (let i = 0; i < 6; i++) {
      const jpegDataUrl = canvas.toDataURL('image/jpeg', quality);
      const b64 = (jpegDataUrl.split(',')[1] || '').trim();
      if (b64 && b64.length < best.length) {
        best = b64;
      }
      if (b64.length <= TARGET_MAX_STORED || quality < 0.35) {
        break;
      }
      quality -= 0.12;
    }
    return best;
  } catch {
    return rawB64;
  }
}

// 开启图片压缩
export const openImageCompression = () => {
  localStorage.setItem(LOCAL_STORAGE_IMAGE_COMPRESSION, '1');
};

// 关闭图片压缩
export const closeImageCompression = () => {
  localStorage.removeItem(LOCAL_STORAGE_IMAGE_COMPRESSION);
};

// 图片压缩开关
export const isImageCompressionOpen = () => {
  return localStorage.getItem(LOCAL_STORAGE_IMAGE_COMPRESSION) === '1';
};

export async function storeBase64WithCompress(
  b64: string
): Promise<ImageRefValue | null> {
  if (!b64) {
    return null;
  }
  // 是否开启图片压缩
  const open = isImageCompressionOpen();
  if (!open) {
    return storeBase64(b64);
  }
  const processed = await compressBase64IfNeeded(b64);
  return storeBase64(processed);
}

export function normalizeFormat(format: any[]): any[] {
  if (!Array.isArray(format)) {
    return format;
  }
  return format.map(it => {
    if (!it || typeof it !== 'object') {
      return it;
    }
    const t = it.type;
    if (t === 'Image' || t === 'ImageURL') {
      if (typeof it.value === 'string') {
        const b64 = extractInlineBase64(it.value);
        if (b64) {
          const ref = storeBase64(b64);
          if (ref) {
            return { type: 'ImageRef', value: ref, options: it.options };
          }
        }
      }
    }
    return it;
  });
}

export async function normalizeFormatAsync(format: any[]): Promise<any[]> {
  if (!Array.isArray(format)) {
    return format;
  }
  const out: any[] = [];
  for (const it of format) {
    if (!it || typeof it !== 'object') {
      out.push(it);
      continue;
    }
    const t = it.type;
    if ((t === 'Image' || t === 'ImageURL') && typeof it.value === 'string') {
      const b64 = extractInlineBase64(it.value);
      if (b64) {
        const ref = await storeBase64WithCompress(b64);
        if (ref) {
          out.push({ type: 'ImageRef', value: ref, options: it.options });
          continue;
        }
      }
    }
    out.push(it);
  }
  return out;
}

// ------------ 工具：base64 -> Blob --------------
function base64ToBlob(base64: string, mime: string): Blob {
  try {
    const byteString = atob(base64);
    const len = byteString.length;
    const u8 = new Uint8Array(len);
    for (let i = 0; i < len; i++) {
      u8[i] = byteString.charCodeAt(i);
    }
    return new Blob([u8], { type: mime });
  } catch {
    return new Blob();
  }
}

// （可选）导出调试接口：释放所有 URL
export function __releaseAllImageBlobs() {
  for (const [, ent] of _blobMap) {
    URL.revokeObjectURL(ent.url);
  }
  _blobMap.clear();
}
