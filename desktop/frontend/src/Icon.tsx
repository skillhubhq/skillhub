const paths: Record<string, string> = {
  inbox: "M4 3h16l2 12v6H2v-6L4 3ZM2 15h6l2 3h4l2-3h6M8 7h8M8 11h8",
  skill: "M12 3 3 8l9 5 9-5-9-5ZM3 12l9 5 9-5M3 16l9 5 9-5",
  code: "m8 6-6 6 6 6m8-12 6 6-6 6m-3-14-2 16",
  chart: "M4 3v17h17M8 15v-4m5 4V7m5 8v-6",
  writing: "m14 4 6 6M4 20l5-1L21 7a2 2 0 0 0-4-4L5 15l-1 5Z",
  workflow: "M4 3h6v6H4zM14 15h6v6h-6zM7 9v9h7M17 15V6h-7",

  terminal: "M3 4h18v16H3zM7 8l4 4-4 4m6 0h4",
  star: "m12 3 2.8 5.7 6.2.9-4.5 4.4 1.1 6.2L12 17.3l-5.6 2.9 1.1-6.2L3 9.6l6.2-.9Z",
  device: "M3 4h18v13H3zM8 21h8m-4-4v4",
  sun: "M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8M12 2v2m0 16v2M2 12h2m16 0h2M5 5l1.4 1.4m11.2 11.2L19 19M5 19l1.4-1.4M17.6 6.4 19 5",
  moon: "M20 15.5A9 9 0 0 1 8.5 4 9 9 0 1 0 20 15.5Z",
  language:
    "M3 5h12M9 2v3m-4 3c1 4 4 6 8 8M13 5c-1 5-4 9-10 11M14 21l4-10 4 10m-6-4h4",
  explore: "M10 3a7 7 0 1 0 0 14 7 7 0 0 0 0-14m5 12 6 6",
  installed: "M12 3v12m-4-4 4 4 4-4M4 16v5h16v-5",
  projects: "M3 7h7l2-3h9v16H3z",
  publish: "M12 4v16M4 12h16",
  submissions: "M5 3h10l4 4v14H5zM9 11h6M9 15h6",
  review: "m4 12 5 5L20 6",
  sources:
    "M9 15 15 9M8 16l-1 1a4 4 0 0 1-6-6l4-4a4 4 0 0 1 6 0m2 10a4 4 0 0 0 6 0l4-4a4 4 0 0 0-6-6l-1 1",
  management: "M4 7h16M4 17h16M8 4v6m8 4v6",
  teams:
    "M8 3a3 3 0 1 0 0 6 3 3 0 0 0 0-6M2 21v-4a6 6 0 0 1 12 0v4M17 4a3 3 0 0 1 0 6m1 4a5 5 0 0 1 4 5v2",
  profile: "M12 3a4 4 0 1 0 0 8 4 4 0 0 0 0-8M4 21a8 8 0 0 1 16 0",
  bookmarks: "M6 3h12v18l-6-4-6 4z",
  bookmark: "M6 3h12v18l-6-4-6 4z",
  heart: "M12 21 3 12a5.5 5.5 0 0 1 9-7 5.5 5.5 0 0 1 9 7z",
  message: "M3 4h18v13H9l-6 4z",
  arrow: "M5 12h14m-5-5 5 5-5 5",
  grid: "M3 3h7v7H3zM14 3h7v7h-7zM3 14h7v7H3zM14 14h7v7h-7z",
};
export function Icon({ name }: { name: string }) {
  return (
    <svg
      className="icon"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d={paths[name] ?? paths.grid} />
    </svg>
  );
}
