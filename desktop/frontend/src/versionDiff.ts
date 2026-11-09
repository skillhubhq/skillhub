export type DiffLine = { kind: "same" | "added" | "removed"; text: string };
// 行数乘积有上限；大文件仅保留共同首尾，避免阻塞渲染线程。
export function diffLines(before: string, after: string): DiffLine[] {
  const a = before.split("\n"),
    b = after.split("\n");
  let start = 0,
    end = 0;
  while (start < a.length && start < b.length && a[start] === b[start]) start++;
  while (
    end < a.length - start &&
    end < b.length - start &&
    a[a.length - 1 - end] === b[b.length - 1 - end]
  )
    end++;
  const result: DiffLine[] = a
    .slice(0, start)
    .map((text) => ({ kind: "same", text }));
  const x = a.slice(start, a.length - end),
    y = b.slice(start, b.length - end);
  if (x.length * y.length > 250000) {
    result.push(
      ...x.map((text) => ({ kind: "removed" as const, text })),
      ...y.map((text) => ({ kind: "added" as const, text })),
    );
  } else {
    const grid = Array.from(
      { length: x.length + 1 },
      () => new Uint32Array(y.length + 1),
    );
    for (let i = x.length - 1; i >= 0; i--)
      for (let j = y.length - 1; j >= 0; j--)
        grid[i][j] =
          x[i] === y[j]
            ? 1 + grid[i + 1][j + 1]
            : Math.max(grid[i + 1][j], grid[i][j + 1]);
    let i = 0,
      j = 0;
    while (i < x.length || j < y.length) {
      if (i < x.length && j < y.length && x[i] === y[j]) {
        result.push({ kind: "same", text: x[i++] });
        j++;
      } else if (
        i < x.length &&
        (j === y.length || grid[i + 1][j] >= grid[i][j + 1])
      )
        result.push({ kind: "removed", text: x[i++] });
      else result.push({ kind: "added", text: y[j++] });
    }
  }
  result.push(
    ...a.slice(a.length - end).map((text) => ({ kind: "same" as const, text })),
  );
  return result;
}
