import classNames from 'classnames';
import { memo } from 'react';

const Select = <T extends { value: string; label: string }>({
  value,
  options,
  onSelect,
  className = ''
}: {
  value: string;
  options: T[];
  onSelect: (data: string) => void;
  className?: string;
}) => {
  return (
    <div className="flex flex-row items-center">
      <select
        value={value}
        onChange={e => {
          const value = e.target.value;
          console.log('Selected value:', value);
          onSelect(value);
        }}
        className={classNames(
          className,
          `px-2 py-1 rounded-md bg-[var(--input-background)] hover:bg-[var(--activityBar-background)] text-[var(--input-foreground)]`
        )}
      >
        {options.map((item, index) => {
          return (
            <option key={index} value={item.value}>
              {item.label}
            </option>
          );
        })}
      </select>
    </div>
  );
};

export default memo(Select);
