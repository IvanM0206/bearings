const PATHS = {
  home: 'M3 11.5 12 4l9 7.5M5.5 10v9.5h4.5V14h4v5.5h4.5V10',
  sections: 'M4 5h16v4H4zM4 11h16v4H4zM4 17h16v2H4z',
  journal: 'M6 3.5h11a1.5 1.5 0 0 1 1.5 1.5v14a1.5 1.5 0 0 1-1.5 1.5H6zM6 3.5v17M9.5 8h6M9.5 12h6',
  path: 'M6 4v6a3 3 0 0 0 3 3h6a3 3 0 0 1 3 3v4M6 4 4 6m2-2 2 2',
  search: 'M10.5 4a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13zM15.5 15.5 20 20',
  plus: 'M12 5v14M5 12h14',
} as const;

export type IconName = keyof typeof PATHS;

export function Icon({ name }: { name: IconName }) {
  return (
    <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
      <path d={PATHS[name]} />
    </svg>
  );
}
