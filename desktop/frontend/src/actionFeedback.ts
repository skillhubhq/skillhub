import { useCallback, useRef, useState } from "react";

// 操作在开始时绑定弹窗；弹窗关闭后返回的失败不能污染其他页面或弹窗。
export function useActionFeedback(explain: (error: unknown) => string) {
  const scopes = useRef<{ id: string }[]>([]);
  const [pending, setPending] = useState(0);
  const [notice, setNotice] = useState({ message: "", scope: "" });
  const registerModal = useCallback((id: string) => {
    const scope = { id };
    scopes.current.push(scope);
    return () => {
      scopes.current = scopes.current.filter((item) => item !== scope);
      setNotice((value) =>
        value.scope === id ? { message: "", scope: "" } : value,
      );
    };
  }, []);
  const setError = useCallback((message: string) => {
    setNotice({ message, scope: "" });
  }, []);
  const run = useCallback(
    async <T>(task: () => Promise<T>): Promise<T | undefined> => {
      const scope = scopes.current.at(-1);
      setPending((value) => value + 1);
      setNotice({ message: "", scope: scope?.id ?? "" });
      try {
        return await task();
      } catch (error) {
        if (!scope || scopes.current.includes(scope))
          setNotice({ message: explain(error), scope: scope?.id ?? "" });
        return undefined;
      } finally {
        setPending((value) => value - 1);
      }
    },
    [explain],
  );
  return {
    pending,
    error: notice.message,
    errorScope: notice.scope,
    setError,
    run,
    registerModal,
  };
}
