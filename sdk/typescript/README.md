# TypeScript protocol SDK

This package exposes the canonical Protobuf messages, enums, service descriptors and a `createHubClients(transport)` factory for the Hub services. The package is built from this repository alongside the Hub.

The CLI and desktop embed the Go client core for source trust, credential persistence, project state and local installation. Integrations using this transport SDK configure their own Connect transport, authentication headers and source policy.

## Build and regenerate

From the repository root:

```sh
make proto
make proto-check
npm --prefix sdk/typescript test
make sdk-smoke
```

Go, Buf 1.72.0 and Node 24 are required for the complete generation/check workflow. CI pins Node 24.21.0. `make proto` installs the lockfile-pinned local JavaScript generator with lifecycle scripts disabled, then generates both Go and TypeScript from `proto/` through `buf.gen.yaml`. Hub/CLI compilation by itself still requires only Go.

`gen/` is generated and checked in. `src/index.ts` provides exports and client construction. `dist/` is compiled locally and ignored by Git. The npm lockfile pins Protobuf-ES / its generator to 2.17.0, Connect to 2.3.0, and TypeScript to 7.0.2. Imported Protovalidate descriptors are generated from the pinned Buf dependency; their notice and license are preserved. Regeneration checks both tracked and newly generated output.

## Integration example

After building this package, link it into an integration as a local package dependency and use a Connect transport suited to that environment. For example, with `@connectrpc/connect-node`:

```ts
import { createConnectTransport } from "@connectrpc/connect-node";
import { createHubClients } from "@skillhubhq/skillhub-protocol";

const clients = createHubClients(createConnectTransport({
  baseUrl: "https://skills.example.com",
  httpVersion: "1.1",
  readMaxBytes: 32 << 20,
  writeMaxBytes: 32 << 20,
  defaultTimeoutMs: 30_000,
}));
const registry = await clients.registry.getRegistry({});
console.log(registry.displayName);
```

Revision and audit sequence fields use `bigint`, byte fields use `Uint8Array`, and timestamps use the generated well-known Protobuf type. Use Protobuf-ES serialization helpers when converting messages to JSON; native `JSON.stringify` does not preserve the Protobuf mapping for these types.

The executable smoke starts an isolated Go Hub and tests both binary Connect and ProtoJSON calls: registry discovery, login/revocation, private access denial, typed error details, unknown enum rejection, immutable idempotent submission, package bytes, timestamps, revision/sequence integers and pagination. It uses disposable credentials and local fixture content. On Windows pass the `.exe` server path to `npm run smoke -- PATH`.

Generation follows [Buf's local plugin workflow](https://buf.build/docs/reference/cli/buf/generate/) and [Connect's generated service descriptors](https://connectrpc.com/docs/web/generating-code/).
