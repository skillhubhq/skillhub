import { spawnSync } from "node:child_process";
import { copyFileSync, existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(fileURLToPath(import.meta.url));
const smoke = process.argv.includes("--native-smoke");
const version = JSON.parse(readFileSync(join(root, "frontend/package.json"), "utf8")).version;
if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(version)) throw new Error("invalid app version");
const bundleVersion = version.split("-")[0];
const appName = smoke ? "SkillHub-smoke" : "SkillHub";
const revisionResult = spawnSync("git", ["rev-parse", "HEAD"], { cwd: root, encoding: "utf8" });
const revision = /^[a-f0-9]{40}$/.test(revisionResult.stdout?.trim()) ? revisionResult.stdout.trim() : "unknown";
const statusResult = spawnSync("git", ["status", "--porcelain"], { cwd: root, encoding: "utf8" });
const buildKind = statusResult.status !== 0 || statusResult.stdout.trim() ? "local" : "source";
const env = { ...process.env };
if (process.platform === "darwin") {
  env.CGO_CFLAGS = "-mmacosx-version-min=13.0";
  env.CGO_LDFLAGS = "-mmacosx-version-min=13.0";
  env.MACOSX_DEPLOYMENT_TARGET = "13.0";
}
function run(command, args, cwd = root) {
  const result = spawnSync(command, args, { cwd, env, stdio: "inherit" });
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status ?? 1);
}
// Go 的 embed 在绑定生成阶段也要求至少一个文件；真正构建将替换此占位。
mkdirSync(join(root, "frontend/dist"), { recursive: true });
if (!existsSync(join(root, "frontend/dist/index.html")))
  writeFileSync(join(root, "frontend/dist/index.html"), "<!doctype html>");
run("go", [
  "tool",
  "wails3",
  "generate",
  "bindings",
  "-ts",
  "-i",
  "-clean=true",
]);
const frontendArgs = ["run", "build", ...(smoke ? ["--", "--mode", "native-smoke"] : [])];
if (process.platform === "win32")
  run("cmd.exe", ["/d", "/s", "/c", "npm", ...frontendArgs], join(root, "frontend"));
else run("npm", frontendArgs, join(root, "frontend"));
if (!smoke) {
  const assetDir = join(root, "frontend/dist/assets");
  for (const name of readdirSync(assetDir)) {
    if (name.endsWith(".js") && readFileSync(join(assetDir, name), "utf8").includes("NativeSmokeReporter"))
      throw new Error("native smoke harness leaked into the production frontend");
  }
}
mkdirSync(join(root, "bin"), { recursive: true });
let output = join(
  root,
  "bin",
  process.platform === "win32" ? `${appName}.exe` : `${appName.toLowerCase()}-app`,
);
if (process.platform === "darwin") {
  const contents = join(root, `bin/${appName}.app/Contents`);
  mkdirSync(join(contents, "MacOS"), { recursive: true });
  mkdirSync(join(contents, "Resources"), { recursive: true });
  copyFileSync(join(root, "assets/SkillHub.icns"), join(contents, "Resources/SkillHub.icns"));
  output = join(contents, "MacOS/SkillHub");
  writeFileSync(
    join(contents, "Info.plist"),
    `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>net.silkage.skillhub${smoke ? ".smoke" : ""}</string>
<key>CFBundleName</key><string>${appName}</string>
<key>CFBundleDisplayName</key><string>${appName}</string>
<key>CFBundleExecutable</key><string>SkillHub</string>
<key>CFBundleIconFile</key><string>SkillHub.icns</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleVersion</key><string>${bundleVersion}</string>
<key>CFBundleShortVersionString</key><string>${bundleVersion}</string>
<key>SkillHubVersion</key><string>${version}</string>
<key>SkillHubRevision</key><string>${revision}</string>
<key>SkillHubBuild</key><string>${buildKind}</string>
<key>LSMinimumSystemVersion</key><string>13.0</string>
<key>NSHighResolutionCapable</key><true/>
</dict></plist>`,
  );
}
if (process.platform === "win32") {
  const arch = env.GOARCH || (process.arch === "arm64" ? "arm64" : "amd64");
  run("go", ["tool", "wails3", "generate", "syso", "-arch", arch,
    "-icon", "assets/SkillHub.ico", "-manifest", "assets/windows.manifest",
    "-out", `rsrc_windows_${arch}.syso`]);
}
const args = ["build", "-tags", smoke ? "production,native_smoke" : "production", "-trimpath", "-o", output];
args.push("-ldflags", `${process.platform === "win32" ? "-H windowsgui " : ""}-X main.appVersion=${version} -X main.appRevision=${revision} -X main.appBuild=${buildKind}`);
args.push(".");
run("go", args);
if (process.platform === "darwin")
  run("codesign", ["--force", "--sign", "-", join(root, `bin/${appName}.app`)]);
if (process.platform === "darwin" && !smoke)
  run("ditto", [
    "-c",
    "-k",
    "--sequesterRsrc",
    "--keepParent",
    join(root, "bin/SkillHub.app"),
    join(root, "bin/SkillHub-macos.zip"),
  ]);
console.log(`Built ${output}`);
