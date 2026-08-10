/**
 * 获取 textarea 光标的像素位置
 * @param textarea 文本框元素
 * @param position 光标位置
 * @returns 光标的 left、top 坐标
 */
export const getCaretCoordinates = (
  textarea: HTMLTextAreaElement,
  position: number
) => {
  // 创建一个隐藏的 div 来模拟 textarea 内容
  const div = document.createElement('div');
  const style = window.getComputedStyle(textarea);

  // 复制 textarea 的样式
  div.style.position = 'absolute';
  div.style.visibility = 'hidden';
  div.style.whiteSpace = 'pre-wrap';
  div.style.wordWrap = 'break-word';
  div.style.font = style.font;
  div.style.padding = style.padding;
  div.style.border = style.border;
  div.style.overflow = style.overflow;
  div.style.width = textarea.offsetWidth + 'px';

  // 设置到光标位置的内容
  div.textContent = textarea.value.substring(0, position);

  // 插入一个 span 来标记光标位置
  const span = document.createElement('span');
  span.textContent = '|'; // 占位符
  div.appendChild(span);

  document.body.appendChild(div);
  const { offsetLeft, offsetTop } = span;
  document.body.removeChild(div);

  // 获取 textarea 在页面上的位置
  const rect = textarea.getBoundingClientRect();
  return {
    left: rect.left + offsetLeft,
    top: rect.top + offsetTop
  };
};
