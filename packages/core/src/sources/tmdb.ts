import { makeTitleId, type MediaKind, type Title } from "../model";
import { cleanText, type FetchFn } from "./util";

const API = "https://api.themoviedb.org/3";
const IMG = "https://image.tmdb.org/t/p/w342";
const ANIMATION_GENRE_ID = 16;

export const TMDB_ATTRIBUTION =
  "This product uses the TMDB API but is not endorsed or certified by TMDB.";

interface TmdbSearchItem {
  id: number;
  media_type: "movie" | "tv" | "person";
  title?: string;
  name?: string;
  original_title?: string;
  original_name?: string;
  release_date?: string;
  first_air_date?: string;
  poster_path?: string | null;
  overview?: string;
  genre_ids?: number[];
  original_language?: string;
  origin_country?: string[];
}

interface TmdbDetail extends Omit<TmdbSearchItem, "media_type" | "genre_ids"> {
  genres?: Array<{ id: number; name: string }>;
  number_of_seasons?: number;
  number_of_episodes?: number;
}

function yearOf(date?: string): number | null {
  if (!date) return null;
  const y = Number(date.slice(0, 4));
  return Number.isFinite(y) && y > 0 ? y : null;
}

/** Japanese animation found on TMDB is shown as anime, so it can be merged with AniList. */
export function isJapaneseAnimation(item: {
  genre_ids?: number[];
  genres?: Array<{ id: number }>;
  original_language?: string;
}): boolean {
  const ids = item.genre_ids ?? item.genres?.map((g) => g.id) ?? [];
  return item.original_language === "ja" && ids.includes(ANIMATION_GENRE_ID);
}

export function mapTmdbItem(
  item: TmdbSearchItem | (TmdbDetail & { media_type: "movie" | "tv" }),
  genreNames: (ids: number[]) => string[],
): Title {
  const type = item.media_type === "tv" ? "tv" : "movie";
  const name = item.title ?? item.name ?? `#${item.id}`;
  const original = item.original_title ?? item.original_name ?? null;
  const detail = item as TmdbDetail;
  const genres = detail.genres
    ? detail.genres.map((g) => g.name)
    : genreNames((item as TmdbSearchItem).genre_ids ?? []);
  const kind: MediaKind = isJapaneseAnimation(item as TmdbSearchItem & TmdbDetail) ? "anime" : type;
  return {
    id: makeTitleId("tmdb", `${type}:${item.id}`),
    source: "tmdb",
    sourceId: `${type}:${item.id}`,
    kind,
    name,
    originalName: original && original !== name ? original : null,
    year: yearOf(item.release_date ?? item.first_air_date),
    posterUrl: item.poster_path ? `${IMG}${item.poster_path}` : null,
    overview: cleanText(item.overview),
    genres,
    format: null,
    seasonCount: type === "tv" ? (detail.number_of_seasons ?? null) : null,
    episodeCount: type === "tv" ? (detail.number_of_episodes ?? null) : null,
  };
}

export interface TmdbClient {
  search(query: string, signal?: AbortSignal): Promise<Title[]>;
  /** `sourceId` is `movie:603` or `tv:1399`. */
  getById(sourceId: string, signal?: AbortSignal): Promise<Title | null>;
}

export interface TmdbOptions {
  /** v3 API key or v4 read access token (starts with "eyJ"). */
  apiKey: string;
  language?: string;
  fetchFn?: FetchFn;
}

export function createTmdbClient({ apiKey, language = "tr-TR", fetchFn = fetch }: TmdbOptions): TmdbClient {
  const isBearer = apiKey.startsWith("eyJ");
  let genreMap: Map<number, string> | null = null;

  async function get<T>(path: string, params: Record<string, string>, signal?: AbortSignal): Promise<T> {
    const qs = new URLSearchParams({ language, ...params });
    if (!isBearer) qs.set("api_key", apiKey);
    const res = await fetchFn(`${API}${path}?${qs}`, {
      headers: {
        Accept: "application/json",
        ...(isBearer ? { Authorization: `Bearer ${apiKey}` } : {}),
      },
      signal,
    });
    if (!res.ok) throw new Error(`TMDB ${res.status}`);
    return (await res.json()) as T;
  }

  async function genres(signal?: AbortSignal) {
    if (genreMap) return genreMap;
    const [m, t] = await Promise.all([
      get<{ genres: Array<{ id: number; name: string }> }>("/genre/movie/list", {}, signal),
      get<{ genres: Array<{ id: number; name: string }> }>("/genre/tv/list", {}, signal),
    ]);
    genreMap = new Map([...m.genres, ...t.genres].map((g) => [g.id, g.name]));
    return genreMap;
  }

  return {
    async search(query, signal) {
      const [data, map] = await Promise.all([
        get<{ results: TmdbSearchItem[] }>(
          "/search/multi",
          { query, include_adult: "false", page: "1" },
          signal,
        ),
        genres(signal).catch(() => new Map<number, string>()),
      ]);
      const names = (ids: number[]) => ids.map((id) => map.get(id)).filter((n): n is string => !!n);
      return data.results
        .filter((r) => r.media_type === "movie" || r.media_type === "tv")
        .map((r) => mapTmdbItem(r, names));
    },
    async getById(sourceId, signal) {
      const [type, id] = sourceId.split(":");
      if ((type !== "movie" && type !== "tv") || !id) return null;
      const detail = await get<TmdbDetail>(`/${type}/${id}`, {}, signal);
      return mapTmdbItem({ ...detail, media_type: type }, () => []);
    },
  };
}
