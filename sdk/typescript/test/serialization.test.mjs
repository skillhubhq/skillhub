import assert from "node:assert/strict";
import { test } from "node:test";
import { create, fromBinary, fromJson, toBinary, toJson } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import {
  AuditEventSchema,
  SubmitReleaseRequestSchema,
  Visibility,
} from "../dist/src/index.js";

test("large uint64 and timestamps retain precision in both encodings", () => {
  const event = create(AuditEventSchema, {
    sequence: 9007199254740993n,
    actorId: "actor",
    createdAt: timestampFromDate(new Date("2026-11-06T00:00:00.123Z")),
  });
  const json = toJson(AuditEventSchema, event);
  assert.equal(json.sequence, "9007199254740993");
  assert.equal(json.createdAt, "2026-11-06T00:00:00.123Z");
  assert.deepEqual(fromJson(AuditEventSchema, json), event);
  assert.deepEqual(fromBinary(AuditEventSchema, toBinary(AuditEventSchema, event)), event);
});

test("archive bytes and enum values survive ProtoJSON and binary", () => {
  const request = create(SubmitReleaseRequestSchema, {
    namespace: "team",
    version: "1.0.0",
    archive: Uint8Array.from([0, 128, 255]),
    visibility: Visibility.GROUPS,
    groupIds: ["group"],
  });
  const json = toJson(SubmitReleaseRequestSchema, request);
  assert.equal(json.archive, "AID/");
  assert.equal(json.visibility, "VISIBILITY_GROUPS");
  assert.deepEqual(fromJson(SubmitReleaseRequestSchema, json), request);
  assert.deepEqual(fromBinary(SubmitReleaseRequestSchema, toBinary(SubmitReleaseRequestSchema, request)), request);
});
