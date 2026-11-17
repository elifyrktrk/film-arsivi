import type { Category, CategoryItem, Rating, Title, WatchStatus } from "./model";

/**
 * Storage contracts. The mobile app implements these on SQLite today; a Supabase
 * implementation (web + sync) will implement the same interfaces later. Business rules
 * live in the services, so implementations only store and fetch.
 */

export interface TitleRepository {
  get(id: string): Promise<Title | null>;
  /** Insert or refresh the cached copy of a title. */
  upsert(title: Title): Promise<void>;
}

export interface ArchiveEntry {
  title: Title;
  rating: Rating;
}

export interface ArchiveQuery {
  status?: WatchStatus | "all";
  sort?: "recent" | "score" | "name";
}

export interface RatingRepository {
  get(titleId: string): Promise<Rating | null>;
  upsert(rating: Rating): Promise<void>;
  delete(titleId: string): Promise<void>;
  /** All rated titles joined with their cached title data. */
  list(query?: ArchiveQuery): Promise<ArchiveEntry[]>;
}

export interface CategoryWithCount extends Category {
  itemCount: number;
  /** Up to 3 poster URLs, newest first, for previews. */
  previewPosters: string[];
}

export interface CategoryRepository {
  list(): Promise<CategoryWithCount[]>;
  get(id: string): Promise<Category | null>;
  insert(category: Category): Promise<void>;
  update(category: Category): Promise<void>;
  /** Deletes the category and its items, never the titles themselves. */
  delete(id: string): Promise<void>;

  listItems(categoryId: string): Promise<Array<CategoryItem & { title: Title }>>;
  getItem(categoryId: string, titleId: string): Promise<CategoryItem | null>;
  insertItem(item: CategoryItem): Promise<void>;
  deleteItem(categoryId: string, titleId: string): Promise<void>;
  /** Highest position in the category, or -1 when empty. */
  maxPosition(categoryId: string): Promise<number>;
  /** IDs of categories that contain the title. */
  categoryIdsForTitle(titleId: string): Promise<string[]>;
}

export interface Repositories {
  titles: TitleRepository;
  ratings: RatingRepository;
  categories: CategoryRepository;
}
