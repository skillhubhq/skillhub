export const signInURL = () =>
  `/login?next=${encodeURIComponent(location.pathname + location.search + location.hash)}`;
