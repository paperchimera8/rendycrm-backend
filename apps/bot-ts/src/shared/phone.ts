export function normalizeRussianPhone(raw: string): string | null {
  const digits = raw.replace(/\D+/g, "");
  if (digits.length === 10) {
    return `7${digits}`;
  }
  if (digits.length === 11 && digits.startsWith("8")) {
    return `7${digits.slice(1)}`;
  }
  if (digits.length === 11 && digits.startsWith("7")) {
    return digits;
  }
  return null;
}
