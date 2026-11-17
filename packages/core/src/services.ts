import { DomainError } from "./errors";
import {
  CATEGORY_DESCRIPTION_MAX,
  CATEGORY_NAME_MAX,
  NOTE_MAX_LENGTH,
  SCORE_MAX,
  SCORE_MIN,
  type Category,
  type CategoryColor,
  type Rating,
  type Title,
  type WatchStatus,
} from "./model";
import type { Repositories } from "./repository";

export interface ServiceDeps {
  repos: Repositories;
  now?: () => Date;
  newId?: () => string;
}

function defaultId(): string {
  const rand = Math.random().toString(36).slice(2, 10);
  return `${Date.now().toString(36)}${rand}`;
}

// ---------- Ratings ----------

export function validateScore(score: number | null): number | null {
  if (score === null) return null;
  if (!Number.isInteger(score) || score < SCORE_MIN || score > SCORE_MAX) {
    throw new DomainError("score_out_of_range");
  }
  return score;
}

export function validateNote(note: string): string {
  const trimmed = note.trim();
  if (trimmed.length > NOTE_MAX_LENGTH) throw new DomainError("note_too_long");
  return trimmed;
}

export interface RatingPatch {
  score?: number | null;
  note?: string;
  status?: WatchStatus | null;
}

/** A rating with no score, note or status carries no information and is removed. */
export function isEmptyRating(r: Pick<Rating, "score" | "note" | "status">): boolean {
  return r.score === null && r.note.length === 0 && r.status === null;
}

export function createRatingService({ repos, now = () => new Date() }: ServiceDeps) {
  return {
    get: (titleId: string) => repos.ratings.get(titleId),

    /**
     * Applies a partial change to the user's record for a title. The title is cached
     * locally so the archive can list it offline. Returns the saved rating, or null
     * when the change cleared everything.
     */
    async update(title: Title, patch: RatingPatch): Promise<Rating | null> {
      const current = await repos.ratings.get(title.id);
      const next: Rating = {
        titleId: title.id,
        score: patch.score !== undefined ? validateScore(patch.score) : (current?.score ?? null),
        note: patch.note !== undefined ? validateNote(patch.note) : (current?.note ?? ""),
        status: patch.status !== undefined ? patch.status : (current?.status ?? null),
        updatedAt: now().toISOString(),
      };
      if (isEmptyRating(next)) {
        if (current) await repos.ratings.delete(title.id);
        return null;
      }
      await repos.titles.upsert(title);
      await repos.ratings.upsert(next);
      return next;
    },

    remove: (titleId: string) => repos.ratings.delete(titleId),
  };
}

// ---------- Categories ----------

export interface CategoryInput {
  name: string;
  color: CategoryColor;
  description?: string;
}

function normalizeName(name: string): string {
  return name.trim().replace(/\s+/g, " ");
}

function nameKey(name: string): string {
  return normalizeName(name).toLocaleLowerCase("tr-TR");
}

export function validateCategoryInput(input: CategoryInput): Required<CategoryInput> {
  const name = normalizeName(input.name);
  if (name.length === 0) throw new DomainError("category_name_required");
  if (name.length > CATEGORY_NAME_MAX) throw new DomainError("category_name_too_long");
  const description = (input.description ?? "").trim();
  if (description.length > CATEGORY_DESCRIPTION_MAX) {
    throw new DomainError("category_description_too_long");
  }
  return { name, color: input.color, description };
}

export function createCategoryService({
  repos,
  now = () => new Date(),
  newId = defaultId,
}: ServiceDeps) {
  async function assertNameFree(name: string, exceptId?: string) {
    const key = nameKey(name);
    const all = await repos.categories.list();
    if (all.some((c) => c.id !== exceptId && nameKey(c.name) === key)) {
      throw new DomainError("category_name_taken");
    }
  }

  async function require(id: string): Promise<Category> {
    const c = await repos.categories.get(id);
    if (!c) throw new DomainError("category_not_found");
    return c;
  }

  return {
    list: () => repos.categories.list(),
    get: (id: string) => repos.categories.get(id),
    listItems: (id: string) => repos.categories.listItems(id),
    categoryIdsForTitle: (titleId: string) => repos.categories.categoryIdsForTitle(titleId),

    async create(input: CategoryInput): Promise<Category> {
      const valid = validateCategoryInput(input);
      await assertNameFree(valid.name);
      const ts = now().toISOString();
      const category: Category = { id: newId(), ...valid, createdAt: ts, updatedAt: ts };
      await repos.categories.insert(category);
      return category;
    },

    async update(id: string, input: CategoryInput): Promise<Category> {
      const current = await require(id);
      const valid = validateCategoryInput(input);
      await assertNameFree(valid.name, id);
      const next: Category = { ...current, ...valid, updatedAt: now().toISOString() };
      await repos.categories.update(next);
      return next;
    },

    async remove(id: string): Promise<void> {
      await repos.categories.delete(id);
    },

    /** Adds a title to the end of a category. Adding twice is a no-op. */
    async addTitle(categoryId: string, title: Title): Promise<void> {
      await require(categoryId);
      if (await repos.categories.getItem(categoryId, title.id)) return;
      await repos.titles.upsert(title);
      const position = (await repos.categories.maxPosition(categoryId)) + 1;
      await repos.categories.insertItem({
        categoryId,
        titleId: title.id,
        position,
        addedAt: now().toISOString(),
      });
    },

    removeTitle: (categoryId: string, titleId: string) =>
      repos.categories.deleteItem(categoryId, titleId),

    /** Puts the title in exactly the given set of categories. */
    async setCategoriesForTitle(title: Title, categoryIds: string[]): Promise<void> {
      const current = new Set(await repos.categories.categoryIdsForTitle(title.id));
      const wanted = new Set(categoryIds);
      for (const id of wanted) if (!current.has(id)) await this.addTitle(id, title);
      for (const id of current) if (!wanted.has(id)) await repos.categories.deleteItem(id, title.id);
    },
  };
}

export type RatingService = ReturnType<typeof createRatingService>;
export type CategoryService = ReturnType<typeof createCategoryService>;
