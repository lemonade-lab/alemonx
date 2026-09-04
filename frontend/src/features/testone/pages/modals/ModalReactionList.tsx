import { useState, useEffect } from 'react';
import { MessageItem, Reaction } from '@testone/typing';
import { Button } from '@testone/ui/Button';
import { Modal } from '../../../../components/Modal';

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
    <Modal
      open
      onClose={onClose}
      ariaLabel="回应用户列表"
      className="testone-modal-surface bg-black/50"
    >
      <div
        className="max-h-[calc(100dvh-32px)] w-[min(500px,calc(100vw-32px))] overflow-y-auto rounded-lg bg-[var(--editor-background)] p-4 shadow-lg"
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
    </Modal>
  );
}
