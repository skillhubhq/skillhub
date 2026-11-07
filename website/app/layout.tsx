import type { Metadata } from "next";
import type { ReactNode } from "react";
import "./global.css";
export const metadata: Metadata = {
  title: { default: "SkillHub Docs", template: "%s · SkillHub Docs" },
  description:
    "Discover, publish and install skills. Run your own SkillHub community.",
  icons: { icon: "/docs/favicon.svg" },
};
export default function Layout({ children }: { children: ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className="docs-shell flex flex-col min-h-screen">{children}</body>
    </html>
  );
}
