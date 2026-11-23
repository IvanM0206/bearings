import { createContext, useContext } from 'react';
import type { Info } from '../api/types';

export interface NotePrefill {
  section?: string;
  topic?: string;
}

export interface App {
  info: Info;
  /** True when edit controls must be hidden. */
  readOnly: boolean;
  /** Increases after every write so that screens reload their data. */
  revision: number;
  bump: () => void;
  /** Shows a short status message (announced to screen readers). */
  flash: (message: string) => void;
  /** Opens the "add note" dialog. */
  openNote: (prefill?: NotePrefill) => void;
}

const AppContext = createContext<App | null>(null);

export const AppProvider = AppContext.Provider;

export function useApp(): App {
  const app = useContext(AppContext);
  if (!app) throw new Error('useApp outside AppProvider');
  return app;
}
