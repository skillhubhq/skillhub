import { Navigation } from "./Navigation";
import { noWorkspaceAccess } from "./workspaceAccess";
import { sessionExpiredEvent } from "./sessionEvents";
import { useState } from "react";
import { flushSync } from "react-dom";
import { createRoot } from "react-dom/client";
import { AccessState } from "./AccessState";
import { UIContext, type UI } from "./ui";

// Retry must recover account-dependent navigation as well as the failed content.
export async function runAccessSmoke(
  check: (condition: unknown, label: string) => void,
) {
  const container = document.createElement("div");
  document.body.append(container);
  const root = createRoot(container);
  function Fixture() {
    const [account, setAccount] = useState(false);
    const [content, setContent] = useState(false);
    const ui: UI = {
      busy: false,
      error: "",
      revision: 0,
      sources: [],
      activeSource: "company",
      access: account ? "member" : "unavailable",
      run: (task) => task(),
      changed: () => {},
      plan: async () => false,
      preview: () => {},
      confirm: () => {},
      navigate: () => {},
      refresh: () => setAccount(true),
    };
    return (
      <UIContext.Provider value={ui}>
        <span data-account>{account ? "Signed in" : "Unavailable"}</span>
        {content ? (
          <span data-content>Skills loaded</span>
        ) : (
          <AccessState
            error="connection refused"
            retry={() => setContent(true)}
          />
        )}
      </UIContext.Provider>
    );
  }
  try {
    flushSync(() => root.render(<Fixture />));
    flushSync(() =>
      container
        .querySelector<HTMLButtonElement>(".access-actions .primary")!
        .click(),
    );
    check(
      container.querySelector("[data-account]")?.textContent === "Signed in" &&
        container.querySelector("[data-content]")?.textContent ===
          "Skills loaded",
      "connection retry refreshes identity and failed content together",
    );
    const ui: UI = {
      busy: false,
      error: "",
      revision: 0,
      sources: [],
      activeSource: "",
      access: "member",
      run: (task) => task(),
      changed: () => {},
      plan: async () => false,
      preview: () => {},
      confirm: () => {},
      navigate: () => {},
      refresh: () => {},
    };
    const labels = () =>
      [...container.querySelectorAll("button")].map((button) =>
        button.textContent?.trim(),
      );
    const renderNavigation = (
      permissions: typeof noWorkspaceAccess,
      page = "discover",
    ) =>
      flushSync(() =>
        root.render(
          <UIContext.Provider value={{ ...ui, workspaceAccess: permissions }}>
            <Navigation page={page} />
          </UIContext.Provider>,
        ),
      );
    renderNavigation(noWorkspaceAccess);
    check(
      !labels().includes("Studio") &&
        !labels().includes("Publish") &&
        labels().includes("Community"),
      "ordinary member navigation excludes privileged entry points",
    );
    renderNavigation({ ...noWorkspaceAccess, review: true }, "review");
    check(
      labels().includes("Reviews") &&
        !labels().includes("My releases") &&
        !labels().includes("Hub settings") &&
        !labels().includes("Publish"),
      "reviewer navigation is limited to review duties",
    );
    renderNavigation({ ...noWorkspaceAccess, publish: true }, "submissions");
    check(
      labels().includes("My releases") &&
        labels().includes("Publish") &&
        !labels().includes("Reviews"),
      "publisher navigation is separate from reviewer duties",
    );
    let expired = "";
    const expire = (event: Event) => {
      expired = (event as CustomEvent<string>).detail;
    };
    window.addEventListener(sessionExpiredEvent, expire);
    try {
      flushSync(() =>
        root.render(
          <UIContext.Provider value={{ ...ui, activeSource: "expired-hub" }}>
            <AccessState error="unauthenticated" />
          </UIContext.Provider>,
        ),
      );
      check(
        expired === "expired-hub",
        "authentication failure invalidates only its source session",
      );
      check(
        !container.textContent?.includes("account provided") &&
          !container.textContent?.includes("Accounts are managed"),
        "sign-in guidance works for automatically provisioned identities",
      );
    } finally {
      window.removeEventListener(sessionExpiredEvent, expire);
    }
  } finally {
    flushSync(() => root.unmount());
    container.remove();
  }
}
