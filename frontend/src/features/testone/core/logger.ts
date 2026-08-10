const isDev = process.env.NODE_ENV !== 'production';
export type LogMethod = (...args: any[]) => void;
function create(method: keyof Console): LogMethod {
  if (!isDev) {
    return () => void 0;
  }
  const fn = (console[method] as any) || console.log;
  return (...args: any[]) => {
    try {
      fn('[ALEMON]', ...args);
    } catch {
      // ignore
    }
  };
}
export const logger = {
  log: create('log'),
  info: create('info'),
  warn: create('warn'),
  error: create('error')
};
export default logger;
