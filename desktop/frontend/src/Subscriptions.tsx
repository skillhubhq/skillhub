import { useEffect, useState } from "react";
import { API, Modal, useUI } from "./ui";
import { useResource, ResourceState } from "./community-ui";
import { t } from "./i18n";
import "./distribution.css";
const subscriptionEvent = "skillhub:subscription-changed";
function subscriptionChanged(coordinate: string) {
  window.dispatchEvent(
    new CustomEvent(subscriptionEvent, { detail: coordinate }),
  );
}
function useSubscriptionRevision(coordinate: string, list = false) {
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const changed = (event: Event) => {
      const updated = (event as CustomEvent<string>).detail;
      if (list ? updated.startsWith(`${coordinate}/`) : updated === coordinate)
        setRevision((value) => value + 1);
    };
    window.addEventListener(subscriptionEvent, changed);
    return () => window.removeEventListener(subscriptionEvent, changed);
  }, [coordinate, list]);
  return revision;
}
export function SubscribeButton({ coordinate }: { coordinate: string }) {
  const ui = useUI();
  const revision = useSubscriptionRevision(coordinate);
  const state = useResource(
    () => API.Subscription(coordinate),
    [coordinate],
    revision,
  );
  return (
    <button
      disabled={ui.busy || state.refreshing}
      aria-pressed={state.value ?? false}
      title={
        state.error ||
        t("Notify me when a version is published", "发布新版本时通知我")
      }
      onClick={() => {
        if (state.error) {
          state.reload();
          return;
        }
        void ui.run(async () => {
          await API.Subscribe(
            coordinate,
            !state.value,
            await API.NewRequestID(),
          );
          subscriptionChanged(coordinate);
        });
      }}
    >
      {state.error
        ? t("Retry subscription", "重试订阅")
        : state.value
          ? t("Following", "已订阅")
          : t("Follow updates", "订阅更新")}
    </button>
  );
}
export function SubscriptionsButton({ source }: { source: string }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button onClick={() => setOpen(true)}>
        {t("Following", "我的订阅")}
      </button>
      {open && (
        <SubscriptionList source={source} close={() => setOpen(false)} />
      )}
    </>
  );
}
function SubscriptionList({
  source,
  close,
}: {
  source: string;
  close: () => void;
}) {
  const ui = useUI();
  const [token, setToken] = useState("");
  const revision = useSubscriptionRevision(source, true);
  const list = useResource(
    () => API.Subscriptions(source, token),
    [source, token],
    revision,
  );
  return (
    <Modal title={t("Following", "我的订阅")} onClose={close} stable="compact">
      <ResourceState {...list} retry={list.reload}>
        {!list.value?.items?.length && (
          <div className="quiet-empty">
            <h3>{t("No followed skills yet", "暂无订阅的技能")}</h3>
            <p>
              {t(
                "Follow skills to receive new-version notifications.",
                "订阅技能后，可以收到新版本通知。",
              )}
            </p>
          </div>
        )}
        <ul className="distribution-list">
          {list.value?.items?.map((v) => (
            <li key={v.namespace + "/" + v.name}>
              <button
                className="distribution-link"
                disabled={!v.available}
                onClick={() => {
                  close();
                  ui.navigate(
                    `discover?${new URLSearchParams({ source, skill: v.namespace + "/" + v.name })}`,
                  );
                }}
              >
                {v.namespace}/{v.name}
                {!v.available && ` · ${t("Unavailable", "不可访问")}`}
              </button>
              <button
                disabled={ui.busy}
                onClick={() =>
                  void ui.run(async () => {
                    await API.Subscribe(
                      `${source}/${v.namespace}/${v.name}`,
                      false,
                      await API.NewRequestID(),
                    );
                    subscriptionChanged(`${source}/${v.namespace}/${v.name}`);
                  })
                }
              >
                {t("Unfollow", "取消订阅")}
              </button>
            </li>
          ))}
        </ul>
        <div className="actions">
          {token && (
            <button onClick={() => setToken("")}>
              {t("First page", "第一页")}
            </button>
          )}
          {list.value?.next_page && (
            <button onClick={() => setToken(list.value!.next_page)}>
              {t("Next page")}
            </button>
          )}
        </div>
      </ResourceState>
    </Modal>
  );
}
