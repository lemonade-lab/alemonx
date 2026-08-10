import classNames from 'classnames';
export function Input(
  props: React.DetailedHTMLProps<
    React.InputHTMLAttributes<HTMLInputElement>,
    HTMLInputElement
  >
) {
  const { className, ...rest } = props;
  return (
    <input
      className={classNames(
        'px-2 py-1 rounded-md flex items-center justify-center',
        'bg-[var(--input-background)]',
        'hover:bg-[var(--input-hoverBackground)]',
        'rounded-md',
        // 'bg-[var(--editor-background)]',
        'border border-[var(--sidebar-border)] focus:border-[var(--button-background)]',
        className
      )}
      {...rest}
    />
  );
}
