let pending: (() => Promise<void>) | undefined;
export function registerDraftSave(save: () => Promise<void>) {
  pending = save;
  return () => {
    if (pending === save) pending = undefined;
  };
}
export function flushPublicationDraft() {
  return pending?.();
}
