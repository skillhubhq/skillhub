import { useState } from "react";
import { API, Modal, useUI } from "./ui";
import { useResource, ResourceState } from "./community-ui";
import { t } from "./i18n";
export function SubscribeButton({ coordinate }: { coordinate: string }) {
  const ui = useUI();
  const state = useResource(() => API.Subscription(coordinate), [coordinate]);
  return (
    <button
      disabled={ui.busy || state.loading}
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
          state.reload();
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
  const list = useResource(
    () => API.Subscriptions(source, token),
    [source, token],
  );
  return (
    <Modal title={t("Following", "我的订阅")} onClose={close} stable>
      <ResourceState {...list} retry={list.reload}>
        {!list.value?.items?.length && (
          <p>
            {t(
              "Follow skills to receive new-version notifications.",
              "订阅技能后，可以收到新版本通知。",
            )}
          </p>
        )}
        <div className="stack">
          {list.value?.items?.map((v) => (
            <div className="section-heading" key={v.namespace + "/" + v.name}>
              <button
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
                    list.reload();
                  })
                }
              >
                {t("Unfollow", "取消订阅")}
              </button>
            </div>
          ))}
        </div>
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
