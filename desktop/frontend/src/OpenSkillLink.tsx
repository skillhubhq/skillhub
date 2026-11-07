import { useEffect, useState } from "react";
import { API, Field, Modal, useUI } from "./ui";
import { t } from "./i18n";
import { OfficialBadge } from "./official";
import type { OpenRequest } from "../bindings/github.com/skillhubhq/skillhub/internal/client/models";

export function OpenSkillLink() {
  const ui = useUI();
  const [request, setRequest] = useState<OpenRequest | null>(null);
  const [alias, setAlias] = useState("web-hub");
  useEffect(() => {
    if (request) return;
    let active = true,
      pending = false;
    const poll = async () => {
      if (pending) return;
      pending = true;
      try {
        const value = await API.NextOpenRequest();
        if (active && value) setRequest(value);
      } catch {
        /* 应用关闭时桥接可能暂时不可用。 */
      } finally {
        pending = false;
      }
    };
    void poll();
    const timer = setInterval(() => void poll(), 1000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [request]);
  if (!request) return null;
  const existing = ui.sources.find((s) => s.url === request.hub);
  return (
    <Modal
      title={t("Open skill from the web", "打开网页中的技能")}
      onClose={() => setRequest(null)}
      stable="compact"
    >
      <div className="stack">
        <p>
          <strong>
            {request.namespace}/{request.name}
          </strong>{" "}
          · v{request.version}
        </p>
        <p className="connection-address">
          {request.hub} <OfficialBadge url={request.hub} />
        </p>
        <p className="muted">
          {existing
            ? t(
                "Open this version using your existing Hub connection. You will choose where to install it next.",
                "使用已有 Hub 连接打开此版本，再选择安装位置。",
              )
            : t(
                "Connect to this Hub to view the skill. Your accounts on other Hubs are not shared. Installation requires a separate confirmation.",
                "连接此 Hub 后查看技能。其他 Hub 的账号不会共享，安装需要另行确认。",
              )}
        </p>
        {!existing && (
          <Field label={t("Connection name", "连接名称")}>
            <input
              value={alias}
              onChange={(e) => setAlias(e.target.value)}
              maxLength={63}
            />
          </Field>
        )}
        <div className="actions">
          <button onClick={() => setRequest(null)}>
            {t("Cancel", "取消")}
          </button>
          <button
            className="primary"
            disabled={
              ui.busy || (!existing && !/^[a-z][a-z0-9-]{0,62}$/.test(alias))
            }
            onClick={() =>
              void ui.run(async () => {
                const source = await API.ConnectOpenRequest(
                  request.hub,
                  existing?.alias || alias,
                );
                ui.refresh();
                ui.navigate(
                  `discover?${new URLSearchParams({ source: source.alias, origin: source.alias, skill: `${request.namespace}/${request.name}`, version: request.version, scope: "current" })}`,
                );
                setRequest(null);
              })
            }
          >
            {existing
              ? t("View skill", "查看技能")
              : t("Connect and view", "连接并查看")}
          </button>
        </div>
      </div>
    </Modal>
  );
}
