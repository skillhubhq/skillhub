import { useEffect, useState } from "react";
import type {
  ConfigTarget,
  SkillConfiguration,
  ConfigVariableStatus,
} from "../bindings/github.com/skillhubhq/skillhub/internal/client/models";
import { API, Empty, Field, Modal, useUI, toolLabel } from "./ui";
import { CopyValue } from "./SkillActions";
import { Select } from "./Select";
import { skillTitle } from "./SkillIdentity";
import { t } from "./i18n";
import "./configuration.css";

const statusLabel = (status: string) =>
  ({
    available: t("Available locally", "本地可读取"),
    missing: t("Not configured", "未配置"),
    "review-required": t("Review required", "需要重新确认"),
    "store-unavailable": t(
      "Unlock system credential store",
      "请解锁系统凭据库",
    ),
  })[status] || status;

export function Configuration() {
  const ui = useUI();
  const [items, setItems] = useState<ConfigTarget[]>([]);
  const [target, setTarget] = useState(
    () =>
      new URLSearchParams(window.location.hash.split("?")[1] || "").get(
        "target",
      ) || "",
  );
  useEffect(() => {
    void ui.run(async () => setItems((await API.ConfigurationTargets()) || []));
  }, [ui.revision]);
  return (
    <>
      <p className="configuration-intro">
        {t(
          "Local to this device. CLI and App share saved configuration; your hubs never receive values.",
          "仅保存在此设备。CLI 与 App 共用配置，Hub 不接收任何配置值。",
        )}
      </p>
      {!items.length ? (
        <Empty title={t("No installed skills", "尚未安装技能")}>
          {t(
            "Install a skill to check its configuration requirements.",
            "安装技能后，可在这里检查它的配置要求。",
          )}
        </Empty>
      ) : (
        <div className="configuration-list">
          {items.map((item) => (
            <article key={item.installation.target}>
              <div>
                <h2>{skillTitle(item.installation)}</h2>
                <p>
                  {item.installation.namespace}/{item.installation.name} ·{" "}
                  {toolLabel(item.installation.tool)} ·{" "}
                  {item.installed
                    ? t("Installed", "已安装")
                    : t("Retained configuration", "保留的配置")}
                </p>
                <small title={item.installation.target}>
                  {item.installation.target}
                </small>
              </div>
              <button onClick={() => setTarget(item.installation.target)}>
                {t("Configure", "配置")}
              </button>
            </article>
          ))}
        </div>
      )}
      {target && (
        <ConfigurationDialog
          target={target}
          onClose={() => {
            setTarget("");
            ui.navigate("configuration");
          }}
        />
      )}
    </>
  );
}

function ConfigurationDialog({
  target,
  onClose,
}: {
  target: string;
  onClose: () => void;
}) {
  const ui = useUI();
  const [report, setReport] = useState<SkillConfiguration>();
  const [using, setUsing] = useState(false);
  const [editing, setEditing] = useState<ConfigVariableStatus>();
  const [mode, setMode] = useState("keychain");
  const [value, setValue] = useState("");
  const refresh = async () => setReport(await API.SkillConfiguration(target));
  useEffect(() => {
    void ui.run(refresh);
  }, [target]);
  const clear = () =>
    ui.confirm(
      t("Clear local configuration?", "清理本地配置？"),
      t(
        "This deletes saved values and system-store secrets for this installation only. Skill files and other installations stay unchanged.",
        "将删除此安装的本地配置值和系统凭据库密钥，保留技能文件和其他安装内容。",
      ),
      async () => {
        await API.ClearSkillConfiguration(target);
        setValue("");
        setEditing(undefined);
        ui.changed(t("Local configuration cleared", "本地配置已清理"));
        if (report?.installed) await refresh();
        else onClose();
      },
      "danger",
    );
  return (
    <Modal
      title={t("Local configuration", "本地配置")}
      stable
      onClose={onClose}
    >
      <div className="configuration-dialog">
        {!using && (
          <>
            <p>
              {t(
                "Save here, then load selected configuration in your own terminal. SkillHub never starts an agent.",
                "在这里保存，再将所选配置加载到自己的终端。SkillHub 不会启动 Agent。",
              )}
            </p>
            <details className="configuration-help">
              <summary>
                {t("Storage and agent access", "保存方式与 Agent 使用")}
              </summary>
              <p className="muted">
                {t(
                  "Environment references use this process's environment. An already-running App or agent may need restarting. This is configuration management, not a runtime sandbox.",
                  "环境变量引用读取当前进程环境，已运行的 App 或 Agent 可能需要重启。此功能管理配置，不提供运行时沙箱。",
                )}
              </p>
            </details>
          </>
        )}
        {!report ? (
          <p role="status">{t("Loading…", "加载中…")}</p>
        ) : (
          <>
            <div className="configuration-context">
              <strong>{skillTitle(report.installation)}</strong>
              <small>
                {report.installation.registry_url} ·{" "}
                {report.installation.namespace}/{report.installation.name} ·{" "}
                {report.installation.version}
              </small>
            </div>
            {report.installed &&
              report.declared &&
              !report.changed &&
              !editing && (
                <button
                  className={
                    using ? "configuration-back" : "primary configuration-use"
                  }
                  onClick={() => setUsing(!using)}
                >
                  {using
                    ? t("Back to configuration", "返回配置")
                    : t("Use in terminal", "在终端中使用")}
                </button>
              )}
            {report.changed && (
              <p role="status" className="notice">
                {t(
                  "This skill's version or origin changed. Clear the previous configuration, review the new requirements, then configure it again.",
                  "技能版本或来源已变化。请清理旧配置、查看新要求后重新配置。",
                )}
              </p>
            )}
            {!report.installed ? (
              <p>
                {t(
                  "This skill is no longer installed. Its retained configuration can be cleared below.",
                  "此技能已卸载，可在下方清理保留的配置。",
                )}
              </p>
            ) : !report.declared ? (
              <p>
                {t(
                  "This skill does not declare configuration requirements. Authors can add the optional skillhub.config.json file; no proprietary format is required to use the skill.",
                  "此技能未声明配置要求。作者可添加可选的 skillhub.config.json；使用技能本身不要求此格式。",
                )}
              </p>
            ) : using ? (
              <EnvironmentGuide target={target} report={report} />
            ) : editing ? null : (
              <div className="configuration-variables">
                {(report.variables || []).map((variable) => (
                  <div key={variable.name} className="configuration-variable">
                    <div>
                      <strong>{variable.name}</strong>
                      <small>
                        {variable.secret
                          ? t("Secret", "密钥")
                          : t("Non-secret", "非敏感配置")}{" "}
                        ·{" "}
                        {variable.required
                          ? t("Required", "必填")
                          : t("Optional", "可选")}
                      </small>
                      <p>{variable.description}</p>
                      <span role="status">{statusLabel(variable.status)}</span>
                    </div>
                    <div className="actions">
                      <button
                        disabled={ui.busy || report.changed}
                        onClick={() => {
                          setEditing(variable);
                          setMode(variable.secret ? "keychain" : "value");
                          setValue("");
                        }}
                      >
                        {t("Set", "设置")}
                      </button>
                      {variable.mode && (
                        <button
                          className="text-danger"
                          disabled={ui.busy}
                          onClick={() =>
                            ui.confirm(
                              t("Remove saved value?", "移除已保存值？"),
                              variable.name,
                              async () => {
                                await API.UnsetSkillConfiguration(
                                  target,
                                  variable.name,
                                );
                                setValue("");
                                setEditing(undefined);
                                await refresh();
                                ui.changed(
                                  t("Saved value removed", "已移除保存值"),
                                );
                              },
                              "danger",
                            )
                          }
                        >
                          {t("Remove", "移除")}
                        </button>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
            {editing && (
              <form
                className="configuration-editor"
                onSubmit={(event) => {
                  event.preventDefault();
                  void ui.run(async () => {
                    const pending = value;
                    setValue("");
                    await API.SetSkillConfiguration(
                      target,
                      editing.name,
                      mode,
                      mode === "environment" ? "" : pending,
                    );
                    setEditing(undefined);
                    await refresh();
                    ui.changed(t("Saved on this device", "已保存在本机"));
                  });
                }}
              >
                <strong>{editing.name}</strong>
                <Field label={t("Storage", "保存方式")}>
                  <Select
                    value={mode}
                    onChange={(event) => {
                      setMode(event.target.value);
                      setValue("");
                    }}
                  >
                    <option value="keychain">
                      {t("System credential store", "系统凭据库")}
                    </option>
                    <option value="environment">
                      {t("Reference environment variable", "引用环境变量")}
                    </option>
                    {!editing.secret && (
                      <option value="value">
                        {t("Local non-secret value", "本地非敏感值")}
                      </option>
                    )}
                  </Select>
                </Field>
                {mode === "environment" ? (
                  <p>
                    {t(
                      "Checks the variable with this exact name; its value is never copied into SkillHub.",
                      "检查此同名环境变量，不将其值复制到 SkillHub。",
                    )}
                  </p>
                ) : (
                  <Field label={t("New value", "新值")}>
                    <input
                      type={mode === "keychain" ? "password" : "text"}
                      autoFocus
                      autoComplete="off"
                      spellCheck={false}
                      maxLength={4096}
                      required
                      value={value}
                      onChange={(event) => setValue(event.target.value)}
                    />
                  </Field>
                )}
                <div className="actions">
                  <button
                    type="button"
                    onClick={() => {
                      setValue("");
                      setEditing(undefined);
                    }}
                  >
                    {t("Cancel", "取消")}
                  </button>
                  <button className="primary" disabled={ui.busy}>
                    {t("Save locally", "保存到本机")}
                  </button>
                </div>
              </form>
            )}
            <div className="actions">
              <button disabled={ui.busy} onClick={() => void ui.run(refresh)}>
                {t("Check again", "重新检查")}
              </button>
              <button
                className="text-danger"
                disabled={ui.busy}
                onClick={clear}
              >
                {t("Clear saved configuration", "清理已保存配置")}
              </button>
            </div>
          </>
        )}
      </div>
    </Modal>
  );
}

function EnvironmentGuide({
  target,
  report,
}: {
  target: string;
  report: SkillConfiguration;
}) {
  const ui = useUI();
  const [shell, setShell] = useState(() =>
    /Windows/i.test(navigator.userAgent) ? "powershell" : "zsh",
  );
  const [instructions, setInstructions] = useState("");
  useEffect(() => {
    let active = true;
    setInstructions("");
    void ui.run(async () => {
      const value = await API.EnvironmentInstructions(target, shell);
      if (active) setInstructions(value);
    });
    return () => {
      active = false;
    };
  }, [target, shell]);
  return (
    <div className="configuration-guide">
      <p>
        {t(
          "Requires the SkillHub CLI on this device. Only the selected variables are loaded.",
          "需要本机已安装 SkillHub CLI，仅加载所选变量。",
        )}
      </p>
      <p className="muted">
        {(report.variables || []).map((v) => v.name).join(" · ")}
      </p>
      <Field label={t("Your terminal shell", "终端 Shell")}>
        <Select
          value={shell}
          onChange={(event) => setShell(event.target.value)}
        >
          <option value="zsh">Zsh</option>
          <option value="bash">Bash</option>
          <option value="powershell">PowerShell 7+</option>
        </Select>
      </Field>
      {instructions && (
        <CopyValue
          label={t(
            "1. Load and check in your terminal",
            "1. 在终端中加载并检查",
          )}
          value={instructions}
        />
      )}
      <p>
        {t(
          "2. Start Codex or Claude Code yourself in the same terminal.",
          "2. 在同一终端自行启动 Codex 或 Claude Code。",
        )}
      </p>
      <CopyValue
        label={t("3. Clear when finished", "3. 使用结束后清理")}
        value="skillhub_env deactivate"
      />
      <details className="configuration-help">
        <summary>{t("Scope and verification", "生效范围与验证")}</summary>
        <p className="muted">
          {t(
            "The check confirms only this process’s environment, not service authentication. No secret appears in these commands. Existing apps are unchanged. Tool environment filters still apply. All code in the receiving process may read its environment; clearing here cannot revoke copies in running processes.",
            "检查仅确认当前进程环境，不代表服务认证成功。这些命令不包含密钥，已运行的 App 不受影响。目标工具仍会应用自己的环境过滤规则。接收进程中的代码可能读取其环境；在此清理无法撤回运行中进程持有的副本。",
          )}
        </p>
      </details>
    </div>
  );
}
