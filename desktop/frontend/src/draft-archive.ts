import { strFromU8, zipSync, type Zippable } from "fflate";

// 仅重新打包已通过预览校验的文件；保留中央目录的权限，避免编辑正文抹掉脚本执行位。
export function repackDraft(
  original: Uint8Array,
  files: Record<string, Uint8Array>,
): Uint8Array {
  const view = new DataView(
    original.buffer,
    original.byteOffset,
    original.byteLength,
  );
  const invalid = () =>
    new Error(
      "Cannot preserve archive metadata; edit this package in the App instead",
    );
  let end = original.length - 22;
  for (; end >= Math.max(0, original.length - 65558); end--) {
    if (
      view.getUint32(end, true) === 0x06054b50 &&
      end + 22 + view.getUint16(end + 20, true) === original.length
    )
      break;
  }
  if (end < 0 || end < original.length - 65558) throw invalid();
  const count = view.getUint16(end + 10, true);
  let offset = view.getUint32(end + 16, true);
  if (
    count === 0xffff ||
    offset === 0xffffffff ||
    view.getUint16(end + 4, true) ||
    view.getUint16(end + 6, true)
  )
    throw invalid();
  const output: Zippable = {};
  for (let i = 0; i < count; i++) {
    if (offset + 46 > end || view.getUint32(offset, true) !== 0x02014b50)
      throw invalid();
    const nameLength = view.getUint16(offset + 28, true);
    const next =
      offset +
      46 +
      nameLength +
      view.getUint16(offset + 30, true) +
      view.getUint16(offset + 32, true);
    if (next > end) throw invalid();
    const name = strFromU8(
      original.subarray(offset + 46, offset + 46 + nameLength),
      !(view.getUint16(offset + 8, true) & 0x800),
    );
    if (Object.hasOwn(files, name)) {
      if (Object.hasOwn(output, name)) throw invalid();
      output[name] = [
        files[name],
        { os: original[offset + 5], attrs: view.getUint32(offset + 38, true) },
      ];
    }
    offset = next;
  }
  if (Object.keys(output).length !== Object.keys(files).length) throw invalid();
  return zipSync(output);
}
