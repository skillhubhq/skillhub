export const sessionExpiredEvent = "skillhub:session-expired";
export function notifySessionExpired(source: string) {
  window.dispatchEvent(
    new CustomEvent(sessionExpiredEvent, { detail: source }),
  );
}
