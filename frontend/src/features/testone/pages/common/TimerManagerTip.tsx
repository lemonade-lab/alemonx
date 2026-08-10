import CloseCircleOutlined from '@ant-design/icons/CloseCircleOutlined';
import FieldTimeOutlined from '@ant-design/icons/FieldTimeOutlined';
/**
 * @returns
 */
const TimerManagerTip = ({
  open,
  count,
  onClose,
  addTask
}: {
  open: boolean;
  count: number;
  onClose: () => void;
  addTask: () => void;
}) => {
  return (
    <div className="absolute z-10 left-1/2 transform -translate-x-1/2 top-[3.2rem] animate__animated animate__fadeIn">
      {open && (
        <div className="flex sm:hidden items-center gap-2 text-xs text-green-500 justify-center animate__animated animate__zoomIn hover-lift">
          <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse"></div>
          <div>任务运行中 ({count})</div>
          <div className="cursor-pointer text-yellow-200 flex items-center">
            <FieldTimeOutlined
              onClick={e => {
                e.stopPropagation();
                addTask();
              }}
            />
          </div>
          <div
            className=" text-red-200 cursor-pointer flex items-center"
            onClick={e => {
              e.stopPropagation();
              onClose();
            }}
          >
            <CloseCircleOutlined />
          </div>
        </div>
      )}
    </div>
  );
};

export default TimerManagerTip;
