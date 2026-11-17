import { describe, expect, it } from "vitest";
import {
  cleanText,
  createAniListClient,
  createCategoryService,
  createMemoryRepositories,
  createRatingService,
  createSearchService,
  DomainError,
  mapAniListMedia,
  mergeSearchResults,
  validateScore,
  type Title,
} from "./index";

function title(over: Partial<Title> & Pick<Title, "id" | "name">): Title {
  const [source, ...rest] = over.id.split(":");
  return {
    source: source as Title["source"],
    sourceId: rest.join(":"),
    kind: "movie",
    originalName: null,
    year: 2000,
    posterUrl: null,
    overview: null,
    genres: [],
    format: null,
    seasonCount: null,
    episodeCount: null,
    ...over,
  };
}

function clock() {
  let t = Date.parse("2026-11-17T10:00:00Z");
  return () => new Date((t += 1000));
}

function ids() {
  let n = 0;
  return () => `c${++n}`;
}

describe("validateScore", () => {
  it("accepts integers 1–10 and null", () => {
    expect(validateScore(1)).toBe(1);
    expect(validateScore(10)).toBe(10);
    expect(validateScore(null)).toBeNull();
  });
  it.each([0, 11, 7.5, -3, Number.NaN])("rejects %s", (v) => {
    expect(() => validateScore(v)).toThrow(DomainError);
  });
});

describe("rating service", () => {
  const movie = title({ id: "tmdb:movie:603", name: "Matrix" });

  it("keeps fields it was not asked to change", async () => {
    const repos = createMemoryRepositories();
    const svc = createRatingService({ repos, now: clock() });
    await svc.update(movie, { score: 9 });
    const r = await svc.update(movie, { status: "watched" });
    expect(r).toMatchObject({ score: 9, status: "watched", note: "" });
    expect(await repos.titles.get(movie.id)).toEqual(movie);
  });

  it("trims notes and removes a record once everything is cleared", async () => {
    const repos = createMemoryRepositories();
    const svc = createRatingService({ repos, now: clock() });
    expect((await svc.update(movie, { note: "  güzel  " }))?.note).toBe("güzel");
    expect(await svc.update(movie, { note: "" })).toBeNull();
    expect(await repos.ratings.get(movie.id)).toBeNull();
  });

  it("lists the archive filtered and sorted", async () => {
    const repos = createMemoryRepositories();
    const svc = createRatingService({ repos, now: clock() });
    const a = title({ id: "tmdb:movie:1", name: "Zeta" });
    const b = title({ id: "tmdb:movie:2", name: "Alfa" });
    await svc.update(a, { score: 6, status: "watched" });
    await svc.update(b, { score: 9, status: "planned" });
    expect((await repos.ratings.list({ sort: "score" })).map((e) => e.title.name)).toEqual(["Alfa", "Zeta"]);
    expect((await repos.ratings.list({ sort: "recent" })).map((e) => e.title.name)).toEqual(["Alfa", "Zeta"]);
    expect((await repos.ratings.list({ status: "watched" })).map((e) => e.title.name)).toEqual(["Zeta"]);
  });
});

describe("category service", () => {
  const setup = () => {
    const repos = createMemoryRepositories();
    return { repos, svc: createCategoryService({ repos, now: clock(), newId: ids() }) };
  };

  it("validates names and rejects duplicates regardless of case and spacing", async () => {
    const { svc } = setup();
    await expect(svc.create({ name: "   ", color: "blue" })).rejects.toMatchObject({ code: "category_name_required" });
    await svc.create({ name: "İyi  Hissettirenler", color: "blue" });
    await expect(svc.create({ name: "iyi hissettirenler", color: "red" })).rejects.toMatchObject({
      code: "category_name_taken",
    });
  });

  it("allows renaming a category to its own name", async () => {
    const { svc } = setup();
    const c = await svc.create({ name: "Twist", color: "blue" });
    await expect(svc.update(c.id, { name: "twist", color: "red" })).resolves.toMatchObject({ name: "twist", color: "red" });
  });

  it("appends items in order and ignores duplicates", async () => {
    const { svc } = setup();
    const c = await svc.create({ name: "Liste", color: "teal" });
    const a = title({ id: "anilist:1", name: "A" });
    const b = title({ id: "anilist:2", name: "B" });
    await svc.addTitle(c.id, a);
    await svc.addTitle(c.id, b);
    await svc.addTitle(c.id, a);
    const items = await svc.listItems(c.id);
    expect(items.map((i) => [i.title.name, i.position])).toEqual([["A", 0], ["B", 1]]);
    expect((await svc.list())[0]?.itemCount).toBe(2);
  });

  it("sets a title's categories and deleting a category keeps the title", async () => {
    const { repos, svc } = setup();
    const c1 = await svc.create({ name: "Bir", color: "teal" });
    const c2 = await svc.create({ name: "İki", color: "pink" });
    const t = title({ id: "tmdb:tv:1399", name: "Dizi" });
    await svc.setCategoriesForTitle(t, [c1.id, c2.id]);
    await svc.setCategoriesForTitle(t, [c2.id]);
    expect(await svc.categoryIdsForTitle(t.id)).toEqual([c2.id]);
    await svc.remove(c2.id);
    expect(await svc.categoryIdsForTitle(t.id)).toEqual([]);
    expect(await repos.titles.get(t.id)).not.toBeNull();
  });

  it("refuses to add to a missing category", async () => {
    const { svc } = setup();
    await expect(svc.addTitle("nope", title({ id: "anilist:1", name: "A" }))).rejects.toMatchObject({
      code: "category_not_found",
    });
  });
});

describe("mergeSearchResults", () => {
  it("drops TMDB anime that AniList also returned", () => {
    const tmdb = [
      title({ id: "tmdb:tv:37854", name: "One Piece", kind: "anime", year: 1999 }),
      title({ id: "tmdb:tv:111110", name: "One Piece", kind: "tv", year: 2023 }),
    ];
    const anilist = [title({ id: "anilist:21", name: "ONE PIECE", kind: "anime", year: 1999 })];
    const ids = mergeSearchResults("one piece", tmdb, anilist).map((t) => t.id);
    expect(ids).toContain("anilist:21");
    expect(ids).toContain("tmdb:tv:111110");
    expect(ids).not.toContain("tmdb:tv:37854");
  });

  it("ranks exact matches first and alternates sources on ties", () => {
    const tmdb = [
      title({ id: "tmdb:movie:1", name: "Dune: Part Two" }),
      title({ id: "tmdb:movie:2", name: "Dune" }),
    ];
    const anilist = [title({ id: "anilist:5", name: "Dune Anime", kind: "anime" })];
    expect(mergeSearchResults("dune", tmdb, anilist).map((t) => t.id)).toEqual([
      "tmdb:movie:2",
      "tmdb:movie:1",
      "anilist:5",
    ]);
  });

  it("matches Turkish characters loosely", () => {
    const tmdb = [title({ id: "tmdb:tv:9", name: "Şahsiyet" })];
    expect(mergeSearchResults("sahsiyet", tmdb, [])[0]?.id).toBe("tmdb:tv:9");
  });
});

describe("search service", () => {
  const anilistOk = {
    search: async () => [title({ id: "anilist:1", name: "Frieren", kind: "anime" })],
    getById: async () => null,
  };

  it("reports TMDB as disabled without a key", async () => {
    const svc = createSearchService({ anilist: anilistOk, tmdb: null });
    const r = await svc.search("frieren");
    expect(r.sources).toEqual({ tmdb: "disabled", anilist: "ok" });
    expect(r.results).toHaveLength(1);
  });

  it("keeps results from one source when the other fails", async () => {
    const tmdb = { search: async () => Promise.reject(new Error("down")), getById: async () => null };
    const r = await createSearchService({ anilist: anilistOk, tmdb }).search("frieren");
    expect(r.sources.tmdb).toBe("error");
    expect(r.results).toHaveLength(1);
  });

  it("fails when no source answers", async () => {
    const anilist = { search: async () => Promise.reject(new Error("down")), getById: async () => null };
    await expect(createSearchService({ anilist, tmdb: null }).search("x y")).rejects.toThrow("search_failed");
  });
});

describe("AniList mapping", () => {
  it("maps media and strips HTML from descriptions", async () => {
    const media = {
      id: 154587,
      title: { romaji: "Sousou no Frieren", english: "Frieren: Beyond Journey's End", native: "葬送のフリーレン" },
      format: "TV",
      episodes: 28,
      seasonYear: 2023,
      startDate: { year: 2023 },
      coverImage: { large: "https://img/x.jpg" },
      description: "Elf mage<br><br>Frieren &amp; friends <i>travel</i>.",
      genres: ["Adventure", "Drama"],
    };
    const t = mapAniListMedia(media);
    expect(t).toMatchObject({
      id: "anilist:154587",
      kind: "anime",
      name: "Frieren: Beyond Journey's End",
      originalName: "Sousou no Frieren",
      episodeCount: 28,
      overview: "Elf mage\n\nFrieren & friends travel.",
    });

    const fetchFn = async () =>
      new Response(JSON.stringify({ data: { Page: { media: [media] } } }), { status: 200 });
    const results = await createAniListClient(fetchFn).search("frieren");
    expect(results[0]?.id).toBe("anilist:154587");
  });

  it("cleanText returns null for empty input", () => {
    expect(cleanText("  <br> ")).toBeNull();
  });
});
