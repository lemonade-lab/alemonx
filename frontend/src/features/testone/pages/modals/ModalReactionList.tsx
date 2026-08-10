import { useState, useEffect } from 'react';
import { MessageItem, Reaction } from '@testone/typing';
import { Button } from '@testone/ui/Button';

type ModalReactionListProps = {
  open: boolean;
  message: MessageItem | null;
  reaction: Reaction | null;
  onClose: () => void;
};

/**
 * 表情回应用户列表模态框
 */
export default function ModalReactionList({
  open,
  message,
  reaction,
  onClose
}: ModalReactionListProps) {
  const [users, setUsers] = useState<string[]>([]);

  useEffect(() => {
    if (reaction) {
      setUsers(reaction.users);
    }
  }, [reaction]);

  if (!open || !message || !reaction) return null;

  return (
    <div
      className="fixed inset-0 z-50 bg-black bg-opacity-50 flex items-center justify-center"
      onClick={onClose}
    >
      <div
        className="bg-[var(--editor-background)] rounded-lg shadow-lg p-4 min-w-[300px] max-w-[500px]"
        onClick={e => e.stopPropagation()}
      >
        <div className="mb-4">
          <h2 className="text-lg font-bold text-[var(--foreground)]">
            <span className="text-2xl mr-2">{reaction.emoji}</span>
            回应用户列表
          </h2>
          <p className="text-xs text-[var(--descriptionForeground)] mt-1">
            共 {users.length} 人给了这个表情回应
          </p>
        </div>

        <div className="mb-4 max-h-[300px] overflow-y-auto">
          {users.length > 0 ? (
            <ul className="space-y-2">
              {users.map((userId, index) => (
                <li
                  key={index}
                  className="px-3 py-2 rounded bg-[var(--panel-background)] text-sm text-[var(--foreground)]"
                >
                  {userId}
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-center text-[var(--descriptionForeground)] py-4">
              暂无用户
            </p>
          )}
        </div>

        <div className="flex gap-2 justify-end">
          <Button onClick={onClose} className="px-4 py-2">
            关闭
          </Button>
        </div>
      </div>
    </div>
  );
}
