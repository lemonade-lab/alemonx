import { Modal, type ModalProps } from 'antd';
import CloseOutlined from '@ant-design/icons/CloseOutlined';
import { Button } from '@testone/ui/Button';

const BaseModal = (
  props: ModalProps & {
    titleIcon: React.ReactNode;
    title: React.ReactNode;
    description?: React.ReactNode;
  }
) => {
  const { onCancel, onOk, cancelText, okText, title, ...reSet } = props;
  return (
    <Modal
      className="testone-modal"
      width={300}
      centered
      onCancel={e => onCancel?.(e)}
      closeIcon={
        <div className="bg-transparent text-[var(--editor-foreground)] rounded p-1 transition-colors">
          <CloseOutlined />
        </div>
      }
      title={
        <div className="flex min-h-8 items-center gap-3 bg-[var(--editor-foreground))]">
          {props.titleIcon}
          <div>
            <h2 className="text-lg font-semibold text-[var(--editor-foreground)] flex items-center gap-2">
              {title}
            </h2>
            {props.description && (
              <p className="text-[11px] mt-0.5 text-[var(--descriptionForeground)]">
                {props.description}
              </p>
            )}
          </div>
        </div>
      }
      footer={
        <div className="flex justify-end gap-3">
          <Button
            type="button"
            onClick={onCancel}
            className="bg-[var(--button-background)] hover:bg-[var(--button-hoverBackground)] text-[var(--button-foreground)] border border-[var(--button-border)]"
          >
            {cancelText || '取消'}
          </Button>
          <Button
            type="button"
            className="bg-[var(--button-background)] hover:bg-[var(--button-hoverBackground)] text-[var(--button-foreground)] border border-[var(--button-border)] flex items-center gap-1"
            onClick={onOk}
          >
            {okText || '确定'}
          </Button>
        </div>
      }
      {...reSet}
    >
      {props.children}
    </Modal>
  );
};

export default BaseModal;
