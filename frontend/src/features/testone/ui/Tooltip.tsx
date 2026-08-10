import React, { useState, ReactNode, useRef, useLayoutEffect } from 'react';
import { createPortal } from 'react-dom';
import classNames from 'classnames';

interface TooltipProps {
  content: ReactNode;
  children: React.ReactElement;
  placement?:
    | 'top'
    | 'bottom'
    | 'left'
    | 'right'
    | 'topright'
    | 'topleft'
    | 'bottomright'
    | 'bottomleft';
  rootClassName?: string;
  className?: string;
  portal?: boolean; // 是否使用 portal 避免被 overflow 裁剪
}

const placementStyle: Record<string, string> = {
  top: 'bottom-full left-1/2 -translate-x-1/2 mb-2',
  bottom: 'top-full left-1/2 -translate-x-1/2 mt-2',
  left: 'right-full top-1/2 -translate-y-1/2 mr-2',
  right: 'left-full top-1/2 -translate-y-1/2 ml-2',
  topright: 'bottom-full left-full mb-2 ml-2',
  topleft: 'bottom-full right-full mb-2 mr-2',
  bottomright: 'top-full left-full mt-2 ml-2',
  bottomleft: 'top-full right-full mt-2 mr-2'
};

export function Tooltip({
  content,
  children,
  placement = 'top',
  rootClassName,
  className,
  portal = false
}: TooltipProps) {
  const [visible, setVisible] = useState(false);
  const anchorRef = useRef<HTMLDivElement | null>(null);
  const [style, setStyle] = useState<React.CSSProperties | undefined>();

  // 克隆子元素，添加事件
  const child = React.cloneElement(children as any, {
    onMouseEnter: (e: any) => {
      (children as any).props?.onMouseEnter?.(e);
      setVisible(true);
    },
    onMouseLeave: (e: any) => {
      (children as any).props?.onMouseLeave?.(e);
      setVisible(false);
    },
    ref: (node: any) => {
      try {
        const r: any = (children as any).ref;
        if (typeof r === 'function') r(node);
        else if (r) r.current = node;
      } catch {}
      anchorRef.current = node;
    }
  });

  useLayoutEffect(() => {
    if (!portal || !visible) return;
    const el = anchorRef.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    const gap = 8;
    let top = 0;
    let left = 0;
    switch (placement) {
      case 'bottom':
        top = rect.bottom + gap;
        left = rect.left + rect.width / 2;
        break;
      case 'left':
        top = rect.top + rect.height / 2;
        left = rect.left - gap;
        break;
      case 'right':
        top = rect.top + rect.height / 2;
        left = rect.right + gap;
        break;
      case 'topright':
        top = rect.top - gap;
        left = rect.right;
        break;
      case 'topleft':
        top = rect.top - gap;
        left = rect.left;
        break;
      case 'bottomright':
        top = rect.bottom + gap;
        left = rect.right;
        break;
      case 'bottomleft':
        top = rect.bottom + gap;
        left = rect.left;
        break;
      case 'top':
      default:
        top = rect.top - gap;
        left = rect.left + rect.width / 2;
    }
    let transform = 'translate(-50%, -100%)';
    if (placement === 'bottom') {
      transform = 'translate(-50%, 0)';
    } else if (placement === 'left') {
      transform = 'translate(-100%, -50%)';
    } else if (placement === 'right') {
      transform = 'translate(0, -50%)';
    } else if (placement === 'bottomright') {
      transform = 'translate(-100%, 0)';
    } else if (placement === 'bottomleft') {
      transform = 'translate(0, 0)';
    } else if (placement === 'topright') {
      transform = 'translate(-100%, -100%)';
    } else if (placement === 'topleft') {
      transform = 'translate(0, -100%)';
    }
    setStyle({
      position: 'fixed',
      top,
      left,
      transform,
      zIndex: 9999,
      pointerEvents: 'none'
    });
  }, [portal, visible, placement]);

  const tooltipInner = visible ? (
    <div
      className={classNames(
        'px-2 py-1 text-sm rounded shadow-lg border',
        'bg-[var(--editorWidget-background)]',
        'text-[var(--editorWidget-foreground)]',
        'border-[var(--editorWidget-border)]',
        'max-w-xs break-words whitespace-normal',
        !portal && 'absolute z-50',
        !portal && placementStyle[placement],
        className
      )}
      style={portal ? style : undefined}
    >
      {content}
    </div>
  ) : null;

  if (portal) {
    return (
      <>
        <div className={classNames('inline-block', rootClassName)}>{child}</div>
        {tooltipInner && createPortal(tooltipInner, document.body)}
      </>
    );
  }

  return (
    <div className={classNames('relative inline-block', rootClassName)}>
      {child}
      {tooltipInner}
    </div>
  );
}
