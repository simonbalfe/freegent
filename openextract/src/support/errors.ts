export function firstLine(error: unknown): string {
  const value = error instanceof Error ? error.message : String(error);
  return value.split("\n")[0] ?? value;
}
