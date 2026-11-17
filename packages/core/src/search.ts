import type { Title } from "./model";
import type { AniListClient } from "./sources/anilist";
import type { TmdbClient } from "./sources/tmdb";
import { normalizeTitle } from "./sources/util";

export type SourceState = "ok" | "disabled" | "error";

export interface SearchResult {
  results: Title[];
  sources: { tmdb: SourceState; anilist: SourceState };
}

function matchScore(query: string, t: Title): number {
  const q = normalizeTitle(query);
  const names = [t.name, t.originalName].filter((n): n is string => !!n).map(normalizeTitle);
  if (names.some((n) => n === q)) return 3;
  if (names.some((n) => n.startsWith(q))) return 2;
  if (names.some((n) => n.includes(q))) return 1;
  return 0;
}

function sameWork(a: Title, b: Title): boolean {
  if (a.year && b.year && Math.abs(a.year - b.year) > 1) return false;
  const an = [a.name, a.originalName].filter((n): n is string => !!n).map(normalizeTitle);
  const bn = [b.name, b.originalName].filter((n): n is string => !!n).map(normalizeTitle);
  return an.some((x) => bn.includes(x));
}

/**
 * Merges results from TMDB and AniList into one list.
 * - Anime that appears in both sources is kept once, from AniList (better anime data).
 * - Ordered by how well the name matches the query, then by each source's own rank,
 *   alternating sources on ties so neither dominates the top of the list.
 */
export function mergeSearchResults(query: string, tmdb: Title[], anilist: Title[]): Title[] {
  const tmdbKept = tmdb.filter((t) => !(t.kind === "anime" && anilist.some((a) => sameWork(t, a))));
  const ranked = [
    ...tmdbKept.map((t, i) => ({ t, i, src: 0 })),
    ...anilist.map((t, i) => ({ t, i, src: 1 })),
  ].map((r) => ({ ...r, score: matchScore(query, r.t) }));
  ranked.sort((a, b) => b.score - a.score || a.i - b.i || a.src - b.src);
  const seen = new Set<string>();
  return ranked.map((r) => r.t).filter((t) => (seen.has(t.id) ? false : (seen.add(t.id), true)));
}

export interface SearchService {
  search(query: string, signal?: AbortSignal): Promise<SearchResult>;
  getTitle(id: string, signal?: AbortSignal): Promise<Title | null>;
  tmdbEnabled: boolean;
}

export function createSearchService(opts: {
  anilist: AniListClient;
  tmdb: TmdbClient | null;
}): SearchService {
  const { anilist, tmdb } = opts;
  return {
    tmdbEnabled: !!tmdb,
    async search(query, signal) {
      const q = query.trim();
      if (q.length < 2) return { results: [], sources: { tmdb: tmdb ? "ok" : "disabled", anilist: "ok" } };
      const [t, a] = await Promise.allSettled([
        tmdb ? tmdb.search(q, signal) : Promise.resolve([]),
        anilist.search(q, signal),
      ]);
      const sources = {
        tmdb: (!tmdb ? "disabled" : t.status === "fulfilled" ? "ok" : "error") as SourceState,
        anilist: (a.status === "fulfilled" ? "ok" : "error") as SourceState,
      };
      if (sources.anilist === "error" && sources.tmdb !== "ok") {
        throw new Error("search_failed");
      }
      return {
        results: mergeSearchResults(
          q,
          t.status === "fulfilled" ? t.value : [],
          a.status === "fulfilled" ? a.value : [],
        ),
        sources,
      };
    },
    async getTitle(id, signal) {
      const [source, ...rest] = id.split(":");
      const sourceId = rest.join(":");
      if (source === "anilist") return anilist.getById(sourceId, signal);
      if (source === "tmdb" && tmdb) return tmdb.getById(sourceId, signal);
      return null;
    },
  };
}
