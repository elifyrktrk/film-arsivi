import { z } from "zod";

/** Where a title's data comes from. */
export const SourceSchema = z.enum(["tmdb", "anilist", "manual"]);
export type Source = z.infer<typeof SourceSchema>;

/** What the user sees as the kind of content. Anime is its own kind, even for anime films. */
export const MediaKindSchema = z.enum(["movie", "tv", "anime"]);
export type MediaKind = z.infer<typeof MediaKindSchema>;

/** Personal watch status. */
export const WatchStatusSchema = z.enum(["watched", "planned", "dropped"]);
export type WatchStatus = z.infer<typeof WatchStatusSchema>;
export const WATCH_STATUSES: readonly WatchStatus[] = WatchStatusSchema.options;

export const SCORE_MIN = 1;
export const SCORE_MAX = 10;
export const NOTE_MAX_LENGTH = 1000;

export const ScoreSchema = z.number().int().min(SCORE_MIN).max(SCORE_MAX);

/**
 * A title (film, series or anime) as stored locally. `id` is stable across sources:
 * `${source}:${sourceId}`, e.g. `tmdb:movie:603`, `anilist:21`, `manual:k3j2...`.
 */
export const TitleSchema = z.object({
  id: z.string().min(1),
  source: SourceSchema,
  sourceId: z.string().min(1),
  kind: MediaKindSchema,
  name: z.string().min(1),
  originalName: z.string().nullable(),
  year: z.number().int().nullable(),
  posterUrl: z.string().nullable(),
  overview: z.string().nullable(),
  genres: z.array(z.string()),
  /** AniList format such as TV, MOVIE, OVA, ONA, SPECIAL. Null for TMDB/manual. */
  format: z.string().nullable(),
  seasonCount: z.number().int().nullable(),
  episodeCount: z.number().int().nullable(),
});
export type Title = z.infer<typeof TitleSchema>;

/** The user's personal record for one title. */
export const RatingSchema = z.object({
  titleId: z.string().min(1),
  score: ScoreSchema.nullable(),
  note: z.string().max(NOTE_MAX_LENGTH),
  status: WatchStatusSchema.nullable(),
  updatedAt: z.string(),
});
export type Rating = z.infer<typeof RatingSchema>;

/** Category colors are stored as keys so each theme can render its own shade. */
export const CategoryColorSchema = z.enum([
  "violet",
  "blue",
  "teal",
  "green",
  "amber",
  "orange",
  "red",
  "pink",
  "slate",
]);
export type CategoryColor = z.infer<typeof CategoryColorSchema>;
export const CATEGORY_COLORS: readonly CategoryColor[] = CategoryColorSchema.options;

export const CATEGORY_NAME_MAX = 40;
export const CATEGORY_DESCRIPTION_MAX = 200;

export const CategorySchema = z.object({
  id: z.string().min(1),
  name: z.string().min(1).max(CATEGORY_NAME_MAX),
  color: CategoryColorSchema,
  description: z.string().max(CATEGORY_DESCRIPTION_MAX),
  createdAt: z.string(),
  updatedAt: z.string(),
});
export type Category = z.infer<typeof CategorySchema>;

export const CategoryItemSchema = z.object({
  categoryId: z.string().min(1),
  titleId: z.string().min(1),
  position: z.number().int().min(0),
  addedAt: z.string(),
});
export type CategoryItem = z.infer<typeof CategoryItemSchema>;

export function makeTitleId(source: Source, sourceId: string): string {
  return `${source}:${sourceId}`;
}
