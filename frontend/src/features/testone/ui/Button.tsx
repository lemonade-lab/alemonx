import classNames from 'classnames';
export function Button(
  props: React.DetailedHTMLProps<
    React.ButtonHTMLAttributes<HTMLButtonElement>,
    HTMLButtonElement
  >
) {
  const { className, onClick, ...rest } = props;
  return (
    <button
      type="button"
      className={classNames(
        'px-2 py-1 rounded-md inline-flex items-center justify-center',
        'bg-[var(--button-background)]',
        'hover:bg-[var(--button-hoverBackground)]',
        'border border-[var(--button-border)]',
        'rounded-md',
        className
      )}
      onClick={e => {
        e.stopPropagation();
        onClick?.(e);
      }}
      {...rest}
    />
  );
}
