import { useState, useEffect } from 'react';
import { Button } from '@testone/ui/Button';
import { MessageItem } from '@testone/typing';
import { DataEnums } from '@testone/typing';
import { Modal } from '../../../../components/Modal';

type ModalEditMessageProps = {
  open: boolean;
  item: MessageItem | null;
  onCancel: () => void;
  onConfirm: (content: DataEnums[]) => void;
};

/**
 * 消息编辑模态框
 */
export default function ModalEditMessage({
  open,
  item,
  onCancel,
  onConfirm
}: ModalEditMessageProps) {
  const [editText, setEditText] = useState('');

  useEffect(() => {
    if (!item) return;
    // 从消息中提取文本内容
    const textContent = Array.isArray(item.data)
      ? item.data.find(d => d.type === 'Text')?.value || ''
      : '';
    setEditText(textContent);
  }, [item]);

  if (!open || !item) return null;

  const handleConfirm = () => {
    // 构造新的消息格式
    const newContent: DataEnums[] = [
      {
        type: 'Text',
        value: editText
      }
    ];
    onConfirm(newContent);
    setEditText('');
  };

  return (
    <Modal
      open
      onClose={onCancel}
      ariaLabel="编辑消息"
      className="testone-modal-surface bg-black/50"
    >
      <div
        className="max-h-[calc(100dvh-32px)] w-[min(600px,calc(100vw-32px))] overflow-y-auto rounded-lg bg-[var(--editor-background)] p-4 shadow-lg"
      >
        <div className="mb-4">
          <h2 className="text-lg font-bold text-[var(--foreground)]">
            编辑消息
          </h2>
          <p className="text-xs text-[var(--descriptionForeground)] mt-1">
            来自 @{item.UserName}
          </p>
        </div>

        <div className="mb-4">
          <textarea
            value={editText}
            onChange={e => setEditText(e.target.value)}
            className="w-full p-2 border border-[var(--editorWidget-border)] rounded bg-[var(--input-background)] text-[var(--foreground)] focus:outline-none focus:border-[var(--editorWidget-background)]"
            rows={4}
            placeholder="输入新的消息内容..."
            maxLength={2000}
          />
          <p className="text-xs text-[var(--descriptionForeground)] mt-1">
            {editText.length} / 2000
          </p>
        </div>

        <div className="flex gap-2 justify-end">
          <Button onClick={onCancel} className="px-4 py-2">
            取消
          </Button>
          <Button
            onClick={handleConfirm}
            disabled={!editText.trim()}
            className="px-4 py-2 bg-[var(--button-background)] text-[var(--button-foreground)]"
          >
            确认编辑
          </Button>
        </div>
      </div>
    </Modal>
  );
}
