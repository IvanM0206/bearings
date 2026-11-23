import { useCallback, useState } from 'react';

export interface Action {
  busy: boolean;
  error: unknown;
  clear: () => void;
  /** Runs `fn`; resolves true when it succeeded. Errors are kept in `error`. */
  run: (fn: () => Promise<unknown>) => Promise<boolean>;
}

/** Tracks one write operation: busy flag and the last error. */
export function useAction(): Action {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(undefined);
  const run = useCallback(async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setError(undefined);
    try {
      await fn();
      return true;
    } catch (e) {
      setError(e ?? new Error('unknown'));
      return false;
    } finally {
      setBusy(false);
    }
  }, []);
  const clear = useCallback(() => setError(undefined), []);
  return { busy, error, clear, run };
}
