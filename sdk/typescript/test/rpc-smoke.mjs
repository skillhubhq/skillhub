import assert from "node:assert/strict";
import { spawn, spawnSync } from "node:child_process";
import { randomBytes, randomUUID, createHash } from "node:crypto";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { resolve, join } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { Code, ConnectError } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-node";
import {
  createHubClients, ErrorDetailSchema, ReleaseStatus, Visibility,
} from "../dist/src/index.js";

const binary = resolve(process.argv[2] ?? "../../bin/skillhub-server");
const data = await mkdtemp(join(tmpdir(), "skillhub-sdk-"));
const password = randomBytes(24).toString("base64url");
let child;
let exited;
try {
  const initialized = spawnSync(binary, ["init", "--data", data, "--password-stdin"], {
    input: password + "\n", stdio: ["pipe", "pipe", "pipe"], timeout: 15000,
  });
  if (initialized.error || initialized.status !== 0) throw new Error("Hub bootstrap failed");
  const listener = createServer();
  await new Promise((accept, reject) => {
    listener.once("error", reject);
    listener.listen(0, "127.0.0.1", accept);
  });
  const origin = `http://127.0.0.1:${listener.address().port}`;
  await new Promise((accept, reject) => listener.close(error => error ? reject(error) : accept()));
  child = spawn(binary, ["serve", "--data", data, "--listen", origin.slice(7), "--public-url", origin], { stdio: "ignore" });
  let spawnError;
  child.once("error", error => { spawnError = error; });
  exited = new Promise(accept => child.once("close", accept));
  const deadline = Date.now() + 15000;
  for (;;) {
    if (spawnError || child.exitCode !== null || child.signalCode !== null) throw new Error("Hub exited before readiness");
    try {
      const response = await fetch(origin + "/healthz", { signal: AbortSignal.timeout(1000) });
      if (response.ok && await response.text() === "ok\n") break;
    } catch { /* 启动期间短暂连接失败可重试。 */ }
    if (Date.now() > deadline) throw new Error("Hub readiness timed out");
    await delay(100);
  }
  const archive = new Uint8Array(await readFile(new URL("./fixtures/sdk-check.zip", import.meta.url)));
  const digest = "sha256:" + createHash("sha256").update(archive).digest("hex");
  let registryId;
  for (const useBinaryFormat of [true, false]) {
    const transportOptions = { baseUrl: origin, httpVersion: "1.1", useBinaryFormat,
      readMaxBytes: 32 << 20, writeMaxBytes: 32 << 20, defaultTimeoutMs: 5000 };
    const anonymous = createHubClients(createConnectTransport(transportOptions));
    const info = await anonymous.registry.getRegistry({});
    assert.equal(info.protocolMajor, 1);
    if (registryId) assert.equal(info.registryId, registryId);
    registryId = info.registryId;
    await assert.rejects(anonymous.catalog.searchSkills({}), error => {
      assert.equal(ConnectError.from(error).code, Code.Unauthenticated);
      assert.equal(ConnectError.from(error).findDetails(ErrorDetailSchema)[0]?.errorCode, "AUTH_REQUIRED");
      return true;
    });
    const login = await anonymous.auth.login({ username: "admin", password });
    assert.equal(login.accessToken.length, 43);
    assert.equal(login.identity.username, "admin");
    assert.ok(login.expiresAt.seconds > BigInt(Math.floor(Date.now() / 1000)));
    const clients = createHubClients(createConnectTransport({ ...transportOptions,
      interceptors: [next => request => {
        request.header.set("Authorization", `Bearer ${login.accessToken}`);
        return next(request);
      }],
    }));
    const namespace = useBinaryFormat ? "sdk-binary" : "sdk-json";
    const created = await clients.admin.putNamespace({ mutation: { idempotencyKey: randomUUID() }, name: namespace, reviewRequired: false });
    assert.equal(created.namespace.revision, 1n);
    await assert.rejects(clients.publishing.submitRelease({
      mutation: { idempotencyKey: randomUUID() }, namespace, version: "1.0.0", archive, visibility: 999,
    }), error => {
      assert.equal(ConnectError.from(error).code, Code.InvalidArgument);
      assert.equal(ConnectError.from(error).findDetails(ErrorDetailSchema)[0]?.errorCode, "INVALID_REQUEST");
      return true;
    });
    const request = { mutation: { idempotencyKey: randomUUID() }, namespace, version: "1.0.0", archive, visibility: Visibility.MEMBERS };
    const submitted = await clients.publishing.submitRelease(request);
    assert.equal(submitted.release.status, ReleaseStatus.PUBLISHED);
    const replay = await clients.publishing.submitRelease(request);
    assert.equal(replay.release.releaseId, submitted.release.releaseId);
    const downloaded = await clients.catalog.downloadRelease({ namespace, name: "sdk-check", version: "1.0.0" });
    assert.equal(downloaded.digest, digest);
    assert.equal(Buffer.compare(Buffer.from(downloaded.archive), Buffer.from(archive)), 0);
    const conditional = await clients.catalog.downloadRelease({ namespace, name: "sdk-check", version: "1.0.0", knownDigest: digest });
    assert.equal(conditional.notModified, true);
    assert.equal(conditional.archive.length, 0);
    assert.equal(conditional.digest, digest);
    assert.equal(conditional.release.releaseId, submitted.release.releaseId);
    const audit = await clients.admin.listAudit({ page: { pageSize: 1 } });
    assert.equal(audit.events.length, 1);
    assert.equal(typeof audit.events[0].sequence, "bigint");
    assert.ok(audit.events[0].createdAt.seconds > 0n);
    assert.ok(audit.page.nextPageToken);
    const next = await clients.admin.listAudit({ page: { pageSize: 1, pageToken: audit.page.nextPageToken } });
    assert.equal(next.events.length, 1);
    assert.notEqual(next.events[0].sequence, audit.events[0].sequence);
    await clients.auth.logout({});
    await assert.rejects(clients.auth.getIdentity({}), error => ConnectError.from(error).code === Code.Unauthenticated);
    await assert.rejects(clients.catalog.downloadRelease({ namespace, name: "sdk-check", version: "1.0.0", knownDigest: digest }), error => ConnectError.from(error).code === Code.Unauthenticated);
  }
  console.log("PASS: generated TypeScript SDK ↔ Go Hub, binary + ProtoJSON, credentials, typed errors, immutable publication, conditional downloads, bytes, timestamps, uint64 and cursors");
} finally {
  if (child && child.exitCode === null && child.signalCode === null) {
    child.kill("SIGTERM");
    const timer = setTimeout(() => child.kill("SIGKILL"), 5000);
    timer.unref();
    await exited;
    clearTimeout(timer);
  }
  await rm(data, { recursive: true, force: true });
}
