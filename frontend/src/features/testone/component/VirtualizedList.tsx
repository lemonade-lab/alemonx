import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState
} from 'react';

export interface VirtualizedListProps<T> {
  items: T[];
  render: (item: T, index: number) => React.ReactNode;
  itemKey: (item: T, index: number) => string | number;
  estimateHeight?: number; // 预估高度
  overscan?: number; // 额外缓冲
  className?: string;
  onRangeChange?: (start: number, end: number) => void;
}

interface ItemMeta {
  h: number;
  top: number;
}
const DEFAULT_EST = 60;

export function VirtualizedList<T>({
  items,
  render,
  itemKey,
  estimateHeight = DEFAULT_EST,
  overscan = 6,
  className,
  onRangeChange
}: VirtualizedListProps<T>) {
  const ref = useRef<HTMLElement | null>(null);
  const [vh, setVh] = useState(0);
  const [st, setSt] = useState(0);
  const metaRef = useRef<ItemMeta[]>([]);
  const nodeRef = useRef<Record<number, HTMLDivElement | null>>({});

  if (metaRef.current.length !== items.length) {
    const arr: ItemMeta[] = new Array(items.length);
    for (let i = 0; i < items.length; i++) {
      const p = metaRef.current[i - 1];
      const h = metaRef.current[i]?.h || estimateHeight;
      arr[i] = { h, top: p ? p.top + p.h : 0 };
    }
    metaRef.current = arr;
  }

  const total = useMemo(() => {
    const last = metaRef.current[metaRef.current.length - 1];
    return last ? last.top + last.h : 0;
  }, [items.length]);

  const findStart = useCallback((scroll: number) => {
    let l = 0,
      r = metaRef.current.length - 1,
      ans = 0;
    while (l <= r) {
      const m = (l + r) >> 1;
      const it = metaRef.current[m];
      if (it.top + it.h > scroll) {
        ans = m;
        r = m - 1;
      } else l = m + 1;
    }
    return ans;
  }, []);

  const range = useMemo(() => {
    if (!vh) return { start: 0, end: items.length - 1 };
    const start = findStart(st);
    let end = start;
    const max = st + vh;
    while (end < items.length && metaRef.current[end].top < max) end++;
    end = Math.min(end + overscan, items.length - 1);
    const realStart = Math.max(0, start - overscan);
    return { start: realStart, end };
  }, [st, vh, items.length, overscan, findStart]);

  useEffect(() => {
    onRangeChange?.(range.start, range.end);
  }, [range, onRangeChange]);

  const onScroll = useCallback(() => {
    if (!ref.current) return;
    setSt(ref.current.scrollTop);
  }, []);

  useEffect(() => {
    if (!ref.current) return;
    const el = ref.current;
    setVh(el.clientHeight);
    el.addEventListener('scroll', onScroll, { passive: true });
    const ro = new ResizeObserver(() => setVh(el.clientHeight));
    ro.observe(el);
    return () => {
      el.removeEventListener('scroll', onScroll);
      ro.disconnect();
    };
  }, [onScroll]);

  useEffect(() => {
    for (let i = range.start; i <= range.end; i++) {
      const n = nodeRef.current[i];
      if (n) {
        const h = n.getBoundingClientRect().height;
        if (h > 0 && h !== metaRef.current[i].h) {
          metaRef.current[i].h = h;
          for (let j = i; j < metaRef.current.length; j++) {
            const p = metaRef.current[j - 1];
            metaRef.current[j].top = j === 0 ? 0 : p.top + p.h;
          }
        }
      }
    }
  }, [range.start, range.end]);

  const children: React.ReactNode[] = [];
  for (let i = range.start; i <= range.end; i++) {
    const it = items[i];
    const m = metaRef.current[i];
    children.push(
      <div
        key={itemKey(it, i)}
        ref={(el: HTMLDivElement | null) => {
          nodeRef.current[i] = el;
        }}
        style={{ position: 'absolute', top: m.top, left: 0, right: 0 }}
      >
        {render(it, i)}
      </div>
    );
  }

  return (
    <section
      ref={ref as any}
      className={className || 'overflow-auto relative'}
      style={{ contain: 'strict' }}
    >
      <div style={{ height: total, position: 'relative' }}>{children}</div>
    </section>
  );
}

export default React.memo(VirtualizedList) as typeof VirtualizedList;
