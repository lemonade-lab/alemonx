import classNames from 'classnames';
import React, { useEffect, useRef } from 'react';

export interface PopoverBoxProps {
  open: boolean;
  onClose?: () => void;
  className?: string;
  style?: React.CSSProperties;
  // 是否监听外部点击
  closeOnOutsideClick?: boolean;
  // 是否监听 Esc
  closeOnEsc?: boolean;
  // 打开后是否自动聚焦
  autoFocus?: boolean;
  // 初始聚焦元素选择器（可选）
  focusSelector?: string;
  children: React.ReactNode;
  // 方向，默认左边
  direction?: 'left' | 'right' | 'top' | 'bottom';
}

const PopoverBox: React.FC<PopoverBoxProps> = ({
  open,
  onClose,
  className = '',
  style,
  closeOnOutsideClick = true,
  closeOnEsc = true,
  autoFocus = true,
  focusSelector,
  children,
  // 方向，默认左边
  direction = 'left'
}) => {
  const ref = useRef<HTMLDivElement>(null);

  // 外部点击
  useEffect(() => {
    if (!open || !closeOnOutsideClick) return;
    const handlePointer = (e: PointerEvent) => {
      if (
        ref.current &&
        e.target instanceof Node &&
        !ref.current.contains(e.target)
      ) {
        onClose?.();
      }
    };
    document.addEventListener('pointerdown', handlePointer, true);
    return () =>
      document.removeEventListener('pointerdown', handlePointer, true);
  }, [open, closeOnOutsideClick, onClose]);

  // ESC
  useEffect(() => {
    if (!open || !closeOnEsc) return;
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose?.();
    };
    document.addEventListener('keydown', handleKey);
    return () => document.removeEventListener('keydown', handleKey);
  }, [open, closeOnEsc, onClose]);

  // 自动聚焦
  useEffect(() => {
    if (open && autoFocus) {
      requestAnimationFrame(() => {
        if (focusSelector) {
          const el = ref.current?.querySelector<HTMLElement>(focusSelector);
          el?.focus();
        } else {
          ref.current?.focus();
        }
      });
    }
  }, [open, autoFocus, focusSelector]);

  if (!open) return null;

  return (
    <div
      ref={ref}
      className={classNames(
        'absolute p-1 mx-4 bottom-full  max-h-60 bg-[var(--dropdown-background)] ',
        'border border-[var(--dropdown-border)] rounded-md shadow-lg z-10 outline-none ',
        {
          'left-0': direction === 'left',
          'right-0': direction === 'right',
          'top-0': direction === 'top',
          'bottom-0': direction === 'bottom'
        },
        className
      )}
      style={style}
      tabIndex={-1}
      role="dialog"
      aria-modal="false"
    >
      {children}
    </div>
  );
};

export default PopoverBox;
