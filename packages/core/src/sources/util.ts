export type FetchFn = (input: string, init?: RequestInit) => Promise<Response>;

const ENTITIES: Record<string, string> = {
  "&amp;": "&",
  "&lt;": "<",
  "&gt;": ">",
  "&quot;": '"',
  "&#039;": "'",
  "&#39;": "'",
  "&nbsp;": " ",
};

/** Strips HTML tags and source notes from descriptions; returns null for empty text. */
export function cleanText(text: string | null | undefined): string | null {
  if (!text) return null;
  const out = text
    .replace(/<br\s*\/?>/gi, "\n")
    .replace(/<[^>]+>/g, "")
    .replace(/&[#a-z0-9]+;/gi, (e) => ENTITIES[e.toLowerCase()] ?? e)
    .replace(/\n{3,}/g, "\n\n")
    .trim();
  return out.length > 0 ? out : null;
}

/** Lowercase, accent-free, punctuation-free form used to compare titles across sources. */
export function normalizeTitle(s: string): string {
  return s
    .toLocaleLowerCase("tr-TR")
    .normalize("NFKD")
    .replace(/[̀-ͯ]/g, "")
    .replace(/ı/g, "i")
    .replace(/[^\p{L}\p{N}]+/gu, " ")
    .trim();
}
