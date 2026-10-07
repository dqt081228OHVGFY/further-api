import { useEffect, useRef, useState } from 'react';
import { useWorkspaceRefresh } from '../native-feature-context';

export function message(cause: unknown): string {
  return cause instanceof Error ? cause.message : '请求失败，请稍后重试。';
}
export function useResource<T>(load: () => Promise<T>, dependencies: unknown[] = []) {
  const [data, setData] = useState<T>();
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const generation = useRef(0);
  const pending = useRef<{
    dependencies: unknown[];
    promise: Promise<T>;
  } | null>(null);
  const reload = async () => {
    const current = ++generation.current;
    setLoading(true);
    setError('');
    // A repeated mount or refresh joins the current request. Keep the promise
    // through effect cleanup so React's development replay does not fetch twice.
    const existing = pending.current;
    const matches = existing && dependencies.length === existing.dependencies.length && dependencies.every((value, index) => Object.is(value, existing.dependencies[index]));
    const request = matches ? existing : {
      dependencies: [...dependencies],
      promise: Promise.resolve().then(load)
    };
    pending.current = request;
    try {
      const result = await request.promise;
      if (current === generation.current) setData(result);
    } catch (cause) {
      if (current === generation.current) setError(message(cause));
    } finally {
      if (pending.current === request) pending.current = null;
      if (current === generation.current) setLoading(false);
    }
  };
  useEffect(() => {
    let active = true;
    queueMicrotask(() => { if (active) void reload(); });
    return () => {
      active = false;
      // This is a request generation counter, not a DOM reference.
      // eslint-disable-next-line react-hooks/exhaustive-deps
      generation.current++;
    };
    // The caller supplies the resource identity; every value is compared with Object.is.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, dependencies);
  useWorkspaceRefresh(reload);
  return {
    data,
    loading,
    error,
    reload
  };
}
