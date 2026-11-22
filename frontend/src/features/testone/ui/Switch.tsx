import { useEffect, useState } from 'react';
import classNames from 'classnames';

export type ToggleSwitchProps = {
  value: boolean;
  id?: string;
  name?: string;
  onChange?: (checked: boolean) => void;
  defaultChecked?: boolean;
  hover?: boolean;
  disabled?: boolean;
};

export const Switch = ({
  value,
  name,
  id,
  onChange,
  defaultChecked,
  disabled = false
}: ToggleSwitchProps) => {
  const [checked, setChecked] = useState(value);

  useEffect(() => {
    setChecked(value);
  }, [value]);

  const handleToggle = () => {
    if (disabled) return;
    const newChecked = !checked;
    setChecked(newChecked);
    if (onChange) {
      onChange(newChecked);
    }
  };

  return (
    <div className="relative inline-flex items-center">
      <input
        type="checkbox"
        name={name}
        id={id}
        checked={checked}
        onChange={handleToggle}
        className="peer sr-only"
        disabled={disabled}
        defaultChecked={defaultChecked}
      />
      <div
        aria-hidden="true"
        className={classNames(
          'w-12 h-6 rounded-full flex items-center cursor-pointer transition-all duration-300 border border-transparent forced-colors:border-[color:CanvasText] forced-colors:[forced-color-adjust:none] peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-(--theme-accent)',
          checked ? 'bg-(--theme-accent) forced-colors:bg-[Highlight]' : 'bg-(--theme-control-track) forced-colors:bg-[Canvas]'
        )}
        onClick={e => {
          e.stopPropagation();
          e.preventDefault();
          handleToggle();
        }}
      >
        <div
          className={classNames(
            'w-4 h-4 bg-white rounded-full shadow transform transition-transform duration-300',
            {
              'translate-x-7 forced-colors:bg-[HighlightText]': checked,
              'translate-x-1 forced-colors:bg-[CanvasText]': !checked
            }
          )}
        />
      </div>
    </div>
  );
};
