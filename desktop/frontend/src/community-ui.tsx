import { SkillSymbol, skillTitle } from "./SkillIdentity";
import { AccessState } from "./AccessState";
import {
  useEffect,
  useRef,
  useState,
  type DependencyList,
  type ReactNode,
  type MouseEvent,
  type ComponentProps,
} from "react";
import { API, useUI } from "./ui";
import { t } from "./i18n";
import type {
  Profile,
  Contribution,
} from "../bindings/github.com/skillhubhq/skillhub/internal/community/models";
export type {
  Profile,
  Team,
  Member,
  Contribution,
  Comment,
  Discussion,
  Page,
} from "../bindings/github.com/skillhubhq/skillhub/internal/community/models";
export function useResource<T>(
  fetcher: () => Promise<T>,
  deps: DependencyList,
) {
  const [value, setValue] = useState<T | null>(null),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true),
    [revision, setRevision] = useState(0);
  useEffect(() => {
    let active = true;
    setLoading(true);
    setValue(null);
    setError("");
    fetcher()
      .then((v) => {
        if (active) setValue(v);
      })
      .catch((e) => {
        if (active) setError(String(e.message ?? e));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [...deps, revision]);
  return {
    value,
    setValue,
    error,
    loading,
    reload: () => setRevision((v) => v + 1),
  };
}
export function useMutationKey() {
  const ref = useRef<{ payload: string; key: string } | null>(null);
  return async (payload: unknown) => {
    const value = JSON.stringify(payload);
    if (ref.current?.payload !== value)
      ref.current = { payload: value, key: await API.NewRequestID() };
    return ref.current.key;
  };
}
export function ResourceState({
  loading,
  error,
  retry,
  children,
}: {
  loading: boolean;
  error: string;
  retry: () => void;
  children?: ReactNode;
}) {
  if (loading)
    return (
      <div
        className="loading-skeleton"
        role="status"
        aria-label={t("Loading", "正在加载")}
      >
        <i />
        <i />
        <i />
      </div>
    );
  if (error) return <AccessState error={error} retry={retry} />;
  return <>{children}</>;
}
// 独立方形画布保证字母不继承标题/按钮的行高，也不被窄列挤压。
export function avatarInitials(name: string) {
  const words = name
    .trim()
    .split(/\s+/)
    .filter((word) => /[\p{L}\p{N}]/u.test(word));
  const initials =
    words.length > 1
      ? [...words[0]][0] + [...words[words.length - 1]][0]
      : [...(words[0] ?? "?")].slice(0, 2).join("");
  return [...initials.toUpperCase()].slice(0, 2).join("");
}
export function Avatar({
  name,
  size = "small",
  image,
}: {
  name: string;
  image?: string;
  size?: "small" | "large" | "medium";
}) {
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [image]);
  let hash = 0;
  for (const c of name) hash += c.codePointAt(0) ?? 0;
  return (
    <span
      className={`avatar avatar-${size} tone-${hash % 5}`}
      aria-hidden="true"
    >
      {image && !failed && image.startsWith("data:image/png;base64,") ? (
        <img src={image} alt="" onError={() => setFailed(true)} />
      ) : (
        <svg
          className="avatar-initials"
          viewBox="0 0 40 40"
          focusable="false"
          aria-hidden="true"
        >
          <text x="20" y="20" dy=".35em" textAnchor="middle">
            {avatarInitials(name)}
          </text>
        </svg>
      )}
    </span>
  );
}
export function Person({
  source,
  person,
}: {
  source: string;
  person: Profile;
}) {
  const ui = useUI();
  return (
    <button
      className="person-link"
      onClick={() =>
        ui.navigate(
          `profile?source=${encodeURIComponent(source)}&id=${encodeURIComponent(person.user_id)}`,
        )
      }
    >
      <Avatar name={person.display_name} image={person.avatar} />
      <span>
        {person.display_name}
        {person.username && <small>@{person.username}</small>}
      </span>
    </button>
  );
}
// 卡片正文和留白共享主入口，独立链接/按钮与文本选择保留自己的行为。
export function openCard(event: MouseEvent<HTMLElement>, open: () => void) {
  if (
    (event.target as Element).closest(
      "a,button,input,select,textarea,summary",
    ) ||
    window.getSelection()?.toString()
  )
    return;
  open();
}
export function AppLink({
  to,
  ...props
}: Omit<ComponentProps<"a">, "href" | "onClick"> & { to: string }) {
  const ui = useUI();
  return (
    <a
      {...props}
      href={`#${to}`}
      onClick={(event) => {
        if (
          event.button !== 0 ||
          event.metaKey ||
          event.ctrlKey ||
          event.shiftKey ||
          event.altKey
        )
          return;
        event.preventDefault();
        ui.navigate(to);
      }}
    />
  );
}
export function Contributions({
  source,
  items,
}: {
  source: string;
  items: Contribution[];
}) {
  const ui = useUI();
  return (
    <div className="contribution-grid">
      {items.map((item) => (
        <article
          className="contribution-card"
          key={item.release_id}
          onClick={(event) =>
            openCard(event, () =>
              ui.navigate(
                `discover?${new URLSearchParams({ source, skill: `${item.namespace}/${item.name}`, version: item.version })}`,
              ),
            )
          }
        >
          <div className="card-overline">
            <SkillSymbol icon={item.icon} />
            <span>v{item.version}</span>
          </div>
          <AppLink
            className="card-title"
            to={`discover?${new URLSearchParams({ source, skill: `${item.namespace}/${item.name}`, version: item.version })}`}
          >
            {skillTitle(item)}
          </AppLink>
          <span className="skill-coordinate">
            {item.namespace}/{item.name}
          </span>
          <p>{item.description}</p>
          <Person source={source} person={item.author} />
        </article>
      ))}
    </div>
  );
}
