import { API } from "./ui";

// 仅测试构建：在真实 WebView 的导入弹窗中选择合成报告，再校验 Go/Hub 持久化。
export async function runEvaluationSmoke(
  release: string,
  digest: string,
  check: (value: unknown, label: string) => void,
  wait: (predicate: () => boolean) => Promise<void>,
) {
  const panel = () => document.querySelector(".evaluation-panel")!;
  await wait(() => !!panel()?.querySelector("button"));
  panel().querySelector<HTMLElement>("summary")!.click();
  const button = (name: string) =>
    Array.from(
      document.querySelectorAll<HTMLButtonElement>("dialog[open] button"),
    ).find((b) => b.textContent === name)!;
  panel().querySelector<HTMLButtonElement>("button")!.click();
  await wait(
    () => !!document.querySelector(".evaluation-import input[type=file]"),
  );
  const report = {
    schema_version: "skillhub.evaluation.v1",
    skill_digest: digest,
    suite: {
      schema_version: "skillhub.cases.v1",
      name: "native-check",
      version: "1",
      cases: [
        {
          id: "missing-owner",
          input: "Review next week. Owner unspecified.",
          expected: "State owner unknown",
        },
      ],
    },
    agent: "example-agent/1",
    model: "example-model/1",
    runner: "manual/1",
    settings: "Synthetic input; tools disabled",
    executed_at: "2026-01-01T00:00:00Z",
    results: [
      {
        case_id: "missing-owner",
        status: "pass",
        observed: "Owner unknown",
        baseline: { status: "fail", observed: "Invented owner" },
      },
    ],
  };
  const choose = (body: unknown) => {
    const input = document.querySelector<HTMLInputElement>(
      ".evaluation-import input[type=file]",
    )!;
    const data = new DataTransfer();
    data.items.add(
      new File([JSON.stringify(body)], "report.json", {
        type: "application/json",
      }),
    );
    input.files = data.files;
    input.dispatchEvent(new Event("change", { bubbles: true }));
  };
  choose({
    ...report,
    suite: { ...report.suite, name: { unexpected: "object" } },
  });
  await wait(() => !!document.querySelector(".evaluation-import [role=alert]"));
  check(
    !!button("Import results")?.disabled,
    "untrusted JSON preview fails safely before rendering objects",
  );
  choose(report);
  await wait(
    () =>
      document
        .querySelector(".evaluation-import")
        ?.textContent?.includes("native-check") ?? false,
  );
  check(
    button("Import results").disabled,
    "evidence import requires privacy acknowledgement",
  );
  document
    .querySelector<HTMLInputElement>(".evaluation-consent input")!
    .click();
  await wait(() => !button("Import results").disabled);
  button("Import results").click();
  await wait(
    () =>
      !!panel().querySelector(".evaluation-row") &&
      !document.querySelector(".evaluation-import"),
  );
  const listed = await API.Evaluations("company", release);
  check(
    listed.supported && listed.can_manage && listed.evidence?.length === 1,
    "native import persists exact version evidence",
  );
  check(
    listed.evidence?.[0].document_json === "",
    "evidence lists omit report body",
  );
  panel().querySelector<HTMLButtonElement>(".evaluation-row")!.click();
  await wait(() => !!document.querySelector(".evaluation-report"));
  check(
    document
      .querySelector(".evaluation-notice")
      ?.textContent?.includes("Not independently verified"),
    "evidence clearly distinguishes external claims from Hub verification",
  );
  document.querySelector<HTMLElement>(".evaluation-cases summary")!.click();
  check(
    document
      .querySelector(".evaluation-cases")
      ?.textContent?.includes("Owner unknown"),
    "case input expected and observed results are readable",
  );
  button("Retract report").click();
  await wait(() => !!button("Confirm retraction"));
  button("Confirm retraction").click();
  await wait(
    () =>
      document
        .querySelector("dialog[open]")
        ?.textContent?.includes("This report was retracted") ?? false,
  );
  const removed = await API.Evaluation("company", listed.evidence![0].id);
  check(
    removed.retracted && removed.document_json === "" && removed.agent === "",
    "native retraction removes original and derived report text",
  );
  document
    .querySelector<HTMLButtonElement>(
      'dialog[open] button[aria-label="Close dialog"]',
    )!
    .click();
  await wait(() => !document.querySelector("dialog[open]"));
  panel().querySelector<HTMLElement>("summary")!.click();
}
