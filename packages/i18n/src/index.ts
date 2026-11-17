/**
 * All user-facing text. Add a new language by copying `tr` and translating values;
 * the `Strings` type keeps every language complete.
 */
export const tr = {
  tabs: { search: "Ara", archive: "Arşivim", categories: "Kategoriler" },
  kind: { movie: "Film", tv: "Dizi", anime: "Anime" },
  source: { tmdb: "TMDB", anilist: "AniList", manual: "Elle eklendi" },
  status: {
    watched: "İzledim",
    planned: "İzleyeceğim",
    dropped: "Yarıda bıraktım",
  },
  search: {
    placeholder: "Film, dizi ya da anime ara",
    hintTitle: "Ne izledin, ne izleyeceksin?",
    hintBody: "En az 2 harf yaz. Filmler, diziler ve animeler tek listede gelir.",
    empty: (q: string) => `“${q}” için sonuç yok`,
    emptyBody: "Yazımı kontrol et ya da orijinal adıyla dene.",
    error: "Arama yapılamadı. İnternet bağlantını kontrol edip tekrar dene.",
    retry: "Tekrar dene",
    tmdbDisabled: "Şu an sadece animeler aranıyor. Film ve diziler için TMDB anahtarı ekle.",
    tmdbError: "TMDB'ye ulaşılamadı, sadece anime sonuçları gösteriliyor.",
    anilistError: "AniList'e ulaşılamadı, sadece film ve dizi sonuçları gösteriliyor.",
  },
  detail: {
    loadError: "Bu içerik yüklenemedi.",
    seasons: (n: number) => `${n} sezon`,
    episodes: (n: number) => `${n} bölüm`,
    yourScore: "Senin puanın",
    clearScore: "Puanı kaldır",
    status: "Durum",
    note: "Not",
    notePlaceholder: "Aklında kalan bir şey, kime önerirsin…",
    saveNote: "Notu kaydet",
    noteSaved: "Kaydedildi",
    categories: "Kategoriler",
    addToCategory: "Kategoriye ekle",
    noCategories: "Henüz kategorin yok.",
    createCategory: "Yeni kategori",
    noOverview: "Özet bulunmuyor.",
  },
  archive: {
    title: "Arşivim",
    all: "Tümü",
    sortRecent: "Son eklenen",
    sortScore: "Puan",
    sortName: "Ad",
    emptyTitle: "Arşivin boş",
    emptyBody: "Bir şey ara, puan ver ya da durumunu işaretle. Burada toplanır.",
    goSearch: "Aramaya git",
    emptyFiltered: "Bu filtrede içerik yok.",
  },
  categories: {
    title: "Kategoriler",
    new: "Yeni kategori",
    edit: "Kategoriyi düzenle",
    emptyTitle: "İlk kategorini oluştur",
    emptyBody: "Örneğin “Pazar akşamı kafa dağıtmalık” ya da “Twist'i olanlar”.",
    items: (n: number) => (n === 0 ? "Boş" : `${n} içerik`),
    emptyCategory: "Bu kategori boş. Bir içeriğin detay sayfasından ekleyebilirsin.",
    name: "Ad",
    namePlaceholder: "Pazar akşamı kafa dağıtmalık",
    description: "Açıklama",
    descriptionPlaceholder: "İsteğe bağlı",
    color: "Renk",
    save: "Kaydet",
    create: "Oluştur",
    delete: "Kategoriyi sil",
    deleteConfirm: "Silmek için tekrar dokun",
    remove: "Çıkar",
  },
  errors: {
    score_out_of_range: "Puan 1 ile 10 arasında olmalı.",
    note_too_long: "Not en fazla 1000 karakter olabilir.",
    category_name_required: "Kategoriye bir ad ver.",
    category_name_too_long: "Ad en fazla 40 karakter olabilir.",
    category_name_taken: "Bu adda bir kategorin zaten var.",
    category_description_too_long: "Açıklama en fazla 200 karakter olabilir.",
    category_not_found: "Kategori bulunamadı.",
    title_not_found: "İçerik bulunamadı.",
    generic: "Bir şeyler ters gitti. Tekrar dene.",
  },
  about: {
    title: "Hakkında",
    body: "Kişisel film, dizi ve anime arşivin. Puanların ve kategorilerin sadece bu cihazda saklanır.",
    sources: "Veri kaynakları",
    tmdbAttribution:
      "Film ve dizi verileri TMDB'den alınır. Bu ürün TMDB API'sini kullanır ancak TMDB tarafından onaylanmamış veya sertifikalandırılmamıştır.",
    anilistAttribution: "Anime verileri AniList'ten alınır.",
    tmdbStatusOn: "TMDB bağlı",
    tmdbStatusOff: "TMDB anahtarı yok: film ve dizi araması kapalı.",
  },
} as const;

type Widen<T> = T extends (...args: infer A) => infer R
  ? (...args: A) => R
  : T extends string
    ? string
    : { [K in keyof T]: Widen<T[K]> };

export type Strings = Widen<typeof tr>;
export const strings: Strings = tr;
