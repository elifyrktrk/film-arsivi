import { makeTitleId, type Title } from "../model";
import { cleanText, type FetchFn } from "./util";

const ENDPOINT = "https://graphql.anilist.co";

const MEDIA_FIELDS = `
  id
  title { romaji english native }
  format
  episodes
  seasonYear
  startDate { year }
  coverImage { large }
  description(asHtml: false)
  genres
`;

const SEARCH_QUERY = `
query ($search: String, $perPage: Int) {
  Page(perPage: $perPage) {
    media(search: $search, type: ANIME, isAdult: false, sort: SEARCH_MATCH) { ${MEDIA_FIELDS} }
  }
}`;

const DETAIL_QUERY = `
query ($id: Int) {
  Media(id: $id, type: ANIME) { ${MEDIA_FIELDS} }
}`;

export interface AniListMedia {
  id: number;
  title: { romaji: string | null; english: string | null; native: string | null };
  format: string | null;
  episodes: number | null;
  seasonYear: number | null;
  startDate: { year: number | null } | null;
  coverImage: { large: string | null } | null;
  description: string | null;
  genres: string[] | null;
}

export function mapAniListMedia(m: AniListMedia): Title {
  const name = m.title.english || m.title.romaji || m.title.native || `#${m.id}`;
  const original = m.title.romaji && m.title.romaji !== name ? m.title.romaji : m.title.native;
  return {
    id: makeTitleId("anilist", String(m.id)),
    source: "anilist",
    sourceId: String(m.id),
    kind: "anime",
    name,
    originalName: original && original !== name ? original : null,
    year: m.seasonYear ?? m.startDate?.year ?? null,
    posterUrl: m.coverImage?.large ?? null,
    overview: cleanText(m.description),
    genres: m.genres ?? [],
    format: m.format,
    seasonCount: null,
    episodeCount: m.episodes,
  };
}

export interface AniListClient {
  search(query: string, signal?: AbortSignal): Promise<Title[]>;
  getById(id: string, signal?: AbortSignal): Promise<Title | null>;
}

export function createAniListClient(fetchFn: FetchFn = fetch): AniListClient {
  async function gql<T>(query: string, variables: object, signal?: AbortSignal): Promise<T> {
    const res = await fetchFn(ENDPOINT, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify({ query, variables }),
      signal,
    });
    if (!res.ok) throw new Error(`AniList ${res.status}`);
    const json = (await res.json()) as { data?: T; errors?: unknown[] };
    if (!json.data) throw new Error("AniList: empty response");
    return json.data;
  }

  return {
    async search(query, signal) {
      const data = await gql<{ Page: { media: AniListMedia[] } }>(
        SEARCH_QUERY,
        { search: query, perPage: 15 },
        signal,
      );
      return data.Page.media.map(mapAniListMedia);
    },
    async getById(id, signal) {
      const data = await gql<{ Media: AniListMedia | null }>(
        DETAIL_QUERY,
        { id: Number(id) },
        signal,
      );
      return data.Media ? mapAniListMedia(data.Media) : null;
    },
  };
}
