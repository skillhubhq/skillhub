import { createMDX } from "fumadocs-mdx/next";

export default createMDX()({
  output: "export",
  webpack(config) {
    config.resolve.extensionAlias = { ...config.resolve.extensionAlias, ".js": [".ts", ".tsx", ".js"] };
    return config;
  },
  basePath: "/docs",
  trailingSlash: true,
  poweredByHeader: false,
  images: { unoptimized: true },
});
