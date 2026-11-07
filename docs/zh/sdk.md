# TypeScript 协议 SDK

SDK 导出规范 Protobuf 消息、枚举、服务描述及 `createHubClients(transport)`，随仓库构建。它是传输 SDK，不承担 Go 内核的源信任、凭据持久化、项目状态或本地安装；集成方自行配置认证头、transport 和来源策略。

## 构建与生成 [#build-and-regenerate]

在仓库根目录执行：

```sh
make proto
make proto-check
npm --prefix sdk/typescript test
make sdk-smoke
```

完整流程使用 Go、Buf 1.72.0、Node 24（CI 固定 24.21.0）。`make proto` 禁用生命周期脚本，按 lockfile 安装 JS 生成器并从 Proto 生成 Go/TS。只编 Hub/CLI 仍仅需 Go。

`gen/` 生成并检入，`src/index.ts` 导出并建客户端，`dist/` 本地编译且忽略。锁文件固定 Protobuf-ES/生成器 2.17.0、Connect 2.3.0、TypeScript 7.0.2，保留 Protovalidate 描述的通知/许可证。漂移检查包含新增文件。

## 集成示例 [#integration-example]

构建后作为本地包依赖，选择环境适合的 Connect transport：

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

revision/审计序列使用 `bigint`，字节为 `Uint8Array`，时间用生成的 Protobuf 类型。转 JSON 使用 Protobuf-ES 序列化工具，不用原生 JSON.stringify 代替协议映射。新增 `clients.evaluation` 提供版本证据操作，能力由注册表 `evaluation.v1` 声明。

冒烟启动隔离 Go Hub，验证二进制 Connect/ProtoJSON、发现、登录撤销、私有拒绝、类型错误、未知枚举、不可变幂等提交、包字节、时间、大整数与分页。只使用临时凭据/合成内容；Windows 传 `.exe` 服务路径。生成遵循 [Buf 本地插件](https://buf.build/docs/reference/cli/buf/generate/)及 [Connect 描述](https://connectrpc.com/docs/web/generating-code/)。
