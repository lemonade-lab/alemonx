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
        className="sr-only"
        disabled={disabled}
        defaultChecked={defaultChecked}
      />
      <div
        className={classNames(
          'w-12 h-6 rounded-full flex items-center cursor-pointer transition-all duration-300 border'
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
              'translate-x-7': checked,
              'translate-x-1': !checked
            }
          )}
        />
      </div>
    </div>
  );
};
