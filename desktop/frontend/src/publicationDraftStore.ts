// 浏览器草稿只写同源 IndexedDB，文件和表单不会发送到 Hub。
async function database(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open("skillhub-publication-drafts", 1);
    req.onupgradeneeded = () => req.result.createObjectStore("drafts");
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}
export async function publicationDraft(
  key: string,
  payload?: string,
): Promise<string> {
  const db = await database();
  try {
    return await new Promise((resolve, reject) => {
      const tx = db.transaction(
        "drafts",
        payload === undefined ? "readonly" : "readwrite",
      );
      const store = tx.objectStore("drafts");
      const req =
        payload === undefined
          ? store.get(key)
          : payload
            ? store.put(payload, key)
            : store.delete(key);
      let result = "";
      req.onsuccess = () => {
        if (payload === undefined) result = req.result || "";
      };
      tx.oncomplete = () => resolve(result);
      tx.onabort = () => reject(tx.error);
      tx.onerror = () => reject(tx.error);
    });
  } finally {
    db.close();
  }
}
