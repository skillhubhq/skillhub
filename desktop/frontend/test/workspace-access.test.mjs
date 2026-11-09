import test from "node:test";
import assert from "node:assert/strict";
import {
  loadWorkspaceAccess,
  noWorkspaceAccess,
  canOpenWorkspacePage,
} from "../src/workspaceAccess.ts";
const api = (
  pages = [{ rows: [], next_page: "" }],
  grants = {},
  duties = {},
) => ({
  PlatformAccess: async () => ({
    roles: null,
    can_manage_users: false,
    can_read_audit: false,
    ...duties,
  }),
  AdminList: async (_s, _k, _q, token) => pages[Number(token || 0)],
  NamespaceAccess: async (_s, name) =>
    grants[name] ?? {
      can_publish: false,
      can_review: false,
      can_manage: false,
    },
});
test("ordinary enterprise members have no studio capabilities", async () => {
  const access = await loadWorkspaceAccess(api(), "enterprise", false);
  assert.deepEqual(access, noWorkspaceAccess);
  for (const page of ["publish", "submissions", "review", "management"])
    assert.equal(canOpenWorkspacePage(access, page), false);
  for (const page of ["discover", "teams", "installed", "settings"])
    assert.equal(canOpenWorkspacePage(access, page), true);
});
test("effective grants include later pages and skip archived teams", async () => {
  const access = await loadWorkspaceAccess(
    api(
      [
        {
          rows: [
            { name: "reader", archived: false },
            { name: "archived", archived: true },
          ],
          next_page: "1",
        },
        { rows: [{ name: "reviewer", archived: false }], next_page: "" },
      ],
      {
        reviewer: { can_publish: false, can_review: true, can_manage: false },
        archived: { can_publish: true, can_review: true, can_manage: true },
      },
    ),
    "enterprise",
    false,
  );
  assert.deepEqual(access, { publish: false, review: true, manage: false });
});
test("maintainer, team admin and platform responsibilities stay separate", async () => {
  const pages = [{ rows: [{ name: "team", archived: false }], next_page: "" }];
  for (const capability of ["publish", "review", "manage"]) {
    const grants = {
      team: {
        can_publish: capability === "publish",
        can_review: capability === "review",
        can_manage: capability === "manage",
      },
    };
    assert.deepEqual(
      await loadWorkspaceAccess(api(pages, grants), "hub", false),
      { ...noWorkspaceAccess, [capability]: true },
    );
  }
  for (const duties of [{ can_manage_users: true }, { can_read_audit: true }]) {
    assert.deepEqual(
      await loadWorkspaceAccess(api(undefined, {}, duties), "hub", false),
      { ...noWorkspaceAccess, manage: true },
    );
  }
  assert.deepEqual(await loadWorkspaceAccess(api(), "hub", true), {
    publish: true,
    review: true,
    manage: true,
  });
});
test("unavailable access and malformed pagination cannot grant navigation", async () => {
  const unavailable = api();
  unavailable.PlatformAccess = async () => {
    throw new Error("unavailable");
  };
  await assert.rejects(
    loadWorkspaceAccess(unavailable, "hub", false),
    /unavailable/,
  );
  await assert.rejects(
    loadWorkspaceAccess(
      api([
        { rows: [], next_page: "1" },
        { rows: [], next_page: "1" },
      ]),
      "hub",
      false,
    ),
    /Repeated/,
  );
});

test("unknown roles do not grant management navigation", async () => {
  assert.deepEqual(
    await loadWorkspaceAccess(
      api(undefined, {}, { roles: [999] }),
      "hub",
      false,
    ),
    noWorkspaceAccess,
  );
});
