import { useState, useEffect } from 'react';
import { Button } from '@testone/ui/Button';
import { MessageItem } from '@testone/typing';
import { DataEnums } from '@testone/typing';

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
    <div
      className="fixed inset-0 z-50 bg-black bg-opacity-50 flex items-center justify-center"
      onClick={onCancel}
    >
      <div
        className="bg-[var(--editor-background)] rounded-lg shadow-lg p-4 min-w-[400px] max-w-[600px]"
        onClick={e => e.stopPropagation()}
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
    </div>
  );
}
