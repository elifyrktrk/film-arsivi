import type { Category, CategoryItem, Rating, Title } from "./model";
import type { ArchiveEntry, ArchiveQuery, CategoryWithCount, Repositories } from "./repository";

/**
 * In-memory repositories. Used by tests and as the reference behavior every storage
 * implementation (SQLite, Supabase) must match.
 */
export function createMemoryRepositories(): Repositories {
  const titles = new Map<string, Title>();
  const ratings = new Map<string, Rating>();
  const categories = new Map<string, Category>();
  let items: CategoryItem[] = [];

  const itemsOf = (categoryId: string) =>
    items.filter((i) => i.categoryId === categoryId).sort((a, b) => a.position - b.position);

  return {
    titles: {
      async get(id) {
        return titles.get(id) ?? null;
      },
      async upsert(title) {
        titles.set(title.id, title);
      },
    },
    ratings: {
      async get(titleId) {
        return ratings.get(titleId) ?? null;
      },
      async upsert(rating) {
        ratings.set(rating.titleId, rating);
      },
      async delete(titleId) {
        ratings.delete(titleId);
      },
      async list(query: ArchiveQuery = {}) {
        const { status = "all", sort = "recent" } = query;
        const out: ArchiveEntry[] = [];
        for (const rating of ratings.values()) {
          const title = titles.get(rating.titleId);
          if (!title) continue;
          if (status !== "all" && rating.status !== status) continue;
          out.push({ title, rating });
        }
        return sortArchive(out, sort);
      },
    },
    categories: {
      async list() {
        const out: CategoryWithCount[] = [...categories.values()].map((c) => {
          const own = itemsOf(c.id);
          const previewPosters = [...own]
            .sort((a, b) => b.addedAt.localeCompare(a.addedAt))
            .map((i) => titles.get(i.titleId)?.posterUrl)
            .filter((u): u is string => !!u)
            .slice(0, 3);
          return { ...c, itemCount: own.length, previewPosters };
        });
        return out.sort((a, b) => a.createdAt.localeCompare(b.createdAt));
      },
      async get(id) {
        return categories.get(id) ?? null;
      },
      async insert(c) {
        categories.set(c.id, c);
      },
      async update(c) {
        categories.set(c.id, c);
      },
      async delete(id) {
        categories.delete(id);
        items = items.filter((i) => i.categoryId !== id);
      },
      async listItems(categoryId) {
        return itemsOf(categoryId).flatMap((i) => {
          const title = titles.get(i.titleId);
          return title ? [{ ...i, title }] : [];
        });
      },
      async getItem(categoryId, titleId) {
        return items.find((i) => i.categoryId === categoryId && i.titleId === titleId) ?? null;
      },
      async insertItem(item) {
        items.push(item);
      },
      async deleteItem(categoryId, titleId) {
        items = items.filter((i) => !(i.categoryId === categoryId && i.titleId === titleId));
      },
      async maxPosition(categoryId) {
        return itemsOf(categoryId).reduce((m, i) => Math.max(m, i.position), -1);
      },
      async categoryIdsForTitle(titleId) {
        return items.filter((i) => i.titleId === titleId).map((i) => i.categoryId);
      },
    },
  };
}

export function sortArchive(
  entries: ArchiveEntry[],
  sort: NonNullable<ArchiveQuery["sort"]>,
): ArchiveEntry[] {
  const copy = [...entries];
  if (sort === "score") {
    copy.sort(
      (a, b) =>
        (b.rating.score ?? 0) - (a.rating.score ?? 0) ||
        b.rating.updatedAt.localeCompare(a.rating.updatedAt),
    );
  } else if (sort === "name") {
    copy.sort((a, b) => a.title.name.localeCompare(b.title.name, "tr-TR"));
  } else {
    copy.sort((a, b) => b.rating.updatedAt.localeCompare(a.rating.updatedAt));
  }
  return copy;
}
