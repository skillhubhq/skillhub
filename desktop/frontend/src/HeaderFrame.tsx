import type { ReactNode } from "react";

// 主站、文档和桌面端共用全局头部结构，业务入口由宿主提供。
export function HeaderFrame({
  brand,
  navigation,
  utilities,
}: {
  brand: ReactNode;
  navigation: ReactNode;
  utilities: ReactNode;
}) {
  return (
    <div className="global-navigation">
      {brand}
      {navigation}
      <div className="global-utilities">{utilities}</div>
    </div>
  );
}
