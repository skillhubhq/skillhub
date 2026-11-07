import type * as Desktop from "../bindings/github.com/skillhubhq/skillhub/desktop/desktopservice";
export const isWeb = __WEB__;
export type PlatformAPI = {
  [K in keyof typeof Desktop]: (
    ...args: Parameters<(typeof Desktop)[K]>
  ) => Promise<Awaited<ReturnType<(typeof Desktop)[K]>>>;
};
// 平台适配只改变传输；Web 构建不会加载 Wails 运行时或本地安装能力。
export const API: PlatformAPI = isWeb
  ? (await import("./web-api")).webAPI
  : await import(
      "../bindings/github.com/skillhubhq/skillhub/desktop/desktopservice"
    );
