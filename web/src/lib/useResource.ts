import { useCallback, useEffect, useRef, useState } from 'react';
import { useApp } from '../app/AppContext';

export interface Resource<T> {
  data: T | undefined;
  error: unknown;
  loading: boolean;
  reload: () => void;
  /** Replaces the data with a fresher copy returned by a write. */
  setData: (data: T) => void;
}

interface State<T> {
  token: string | undefined;
  data: T | undefined;
  error: unknown;
}

/**
 * Loads data from the API. It reloads when `deps` (primitive values) change
 * and whenever something in the notebook was written (see
 * AppContext.revision). The previous data stays visible while a reload runs.
 */
export function useResource<T>(load: (signal: AbortSignal) => Promise<T>, deps: readonly unknown[]): Resource<T> {
  const { revision } = useApp();
  const [manual, setManual] = useState(0);
  const token = JSON.stringify([...deps, revision, manual]);
  const [state, setState] = useState<State<T>>({ token: undefined, data: undefined, error: undefined });
  const loadRef = useRef(load);
  useEffect(() => {
    loadRef.current = load;
  });

  useEffect(() => {
    const ctl = new AbortController();
    loadRef
      .current(ctl.signal)
      .then((data) => {
        if (!ctl.signal.aborted) setState({ token, data, error: undefined });
      })
      .catch((error: unknown) => {
        if (!ctl.signal.aborted) setState((s) => ({ token, data: s.data, error }));
      });
    return () => ctl.abort();
  }, [token]);

  const reload = useCallback(() => setManual((n) => n + 1), []);
  const setData = useCallback((data: T) => setState((s) => ({ ...s, data, error: undefined })), []);
  return { data: state.data, error: state.error, loading: state.token !== token, reload, setData };
}
