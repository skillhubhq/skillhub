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
      plan: () => {},
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
  } finally {
    flushSync(() => root.unmount());
    container.remove();
  }
}
