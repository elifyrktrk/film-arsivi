// Package i18n holds every user-facing string of the web app. The shared keys are a
// copy of packages/i18n/src/index.ts (`tr`) and must stay in step with it;
// i18n_test.go checks that every shared string still appears there verbatim.
// Web-only strings live under Web.
package i18n

import "fmt"

type Strings struct {
	Tabs       struct{ Search, Archive, Categories string }
	Kind       map[string]string
	Source     map[string]string
	Status     map[string]string
	Search     SearchStrings
	Detail     DetailStrings
	Archive    ArchiveStrings
	Categories CategoryStrings
	Errors     map[string]string
	About      AboutStrings
	Web        WebStrings
}

type SearchStrings struct {
	Placeholder, HintTitle, HintBody, EmptyBody, Error, Retry, TMDBDisabled, TMDBError, AniListError string
}

func (SearchStrings) Empty(q string) string { return "“" + q + "” için sonuç yok" }

type DetailStrings struct {
	LoadError, YourScore, ClearScore, Status, Note, NotePlaceholder, SaveNote, NoteSaved,
	Categories, AddToCategory, NoCategories, CreateCategory, NoOverview string
}

func (DetailStrings) Seasons(n int) string  { return fmt.Sprintf("%d sezon", n) }
func (DetailStrings) Episodes(n int) string { return fmt.Sprintf("%d bölüm", n) }

type ArchiveStrings struct {
	Title, All, SortRecent, SortScore, SortName, EmptyTitle, EmptyBody, GoSearch, EmptyFiltered string
}

type CategoryStrings struct {
	Title, New, Edit, EmptyTitle, EmptyBody, EmptyCategory, Name, NamePlaceholder, Description,
	DescriptionPlaceholder, Color, Save, Create, Delete, DeleteConfirm, Remove string
}

func (CategoryStrings) Items(n int) string {
	if n == 0 {
		return "Boş"
	}
	return fmt.Sprintf("%d içerik", n)
}

type AboutStrings struct {
	Title, Body, Sources, TMDBAttribution, AniListAttribution, TMDBStatusOn, TMDBStatusOff string
}

// WebStrings are only used by the web app.
type WebStrings struct {
	AppName, About, Theme, ThemeSystem, ThemeLight, ThemeDark, SearchButton, Loading,
	StatusNone, Save, Saved, NotFoundTitle, NotFoundBody, ServerError, Back,
	DeleteTitle, DeleteBody, DeleteYes, Cancel, Sort, Filter, Score, NoScore,
	CategoriesSaved, CategoryNamedHelp, AboutStorage, OpenDetail, Year, Genres, Format,
	SkipToContent, CreateAndAdd string
	Community, Votes, YourScoreShort, RateIt, IMDbAttribution, IMDbAttributionEN string
	// Colors names the category color keys for labels and screen readers.
	Colors map[string]string
}

var TR = func() Strings {
	var s Strings
	s.Tabs.Search, s.Tabs.Archive, s.Tabs.Categories = "Ara", "Arşivim", "Kategoriler"
	s.Kind = map[string]string{"movie": "Film", "tv": "Dizi", "anime": "Anime"}
	s.Source = map[string]string{"tmdb": "TMDB", "anilist": "AniList", "manual": "Elle eklendi"}
	s.Status = map[string]string{"watched": "İzledim", "planned": "İzleyeceğim", "dropped": "Yarıda bıraktım"}
	s.Search = SearchStrings{
		Placeholder:  "Film, dizi ya da anime ara",
		HintTitle:    "Ne izledin, ne izleyeceksin?",
		HintBody:     "En az 2 harf yaz. Filmler, diziler ve animeler tek listede gelir.",
		EmptyBody:    "Yazımı kontrol et ya da orijinal adıyla dene.",
		Error:        "Arama yapılamadı. İnternet bağlantını kontrol edip tekrar dene.",
		Retry:        "Tekrar dene",
		TMDBDisabled: "Şu an sadece animeler aranıyor. Film ve diziler için TMDB anahtarı ekle.",
		TMDBError:    "TMDB'ye ulaşılamadı, sadece anime sonuçları gösteriliyor.",
		AniListError: "AniList'e ulaşılamadı, sadece film ve dizi sonuçları gösteriliyor.",
	}
	s.Detail = DetailStrings{
		LoadError:       "Bu içerik yüklenemedi.",
		YourScore:       "Senin puanın",
		ClearScore:      "Puanı kaldır",
		Status:          "Durum",
		Note:            "Not",
		NotePlaceholder: "Aklında kalan bir şey, kime önerirsin…",
		SaveNote:        "Notu kaydet",
		NoteSaved:       "Kaydedildi",
		Categories:      "Kategoriler",
		AddToCategory:   "Kategoriye ekle",
		NoCategories:    "Henüz kategorin yok.",
		CreateCategory:  "Yeni kategori",
		NoOverview:      "Özet bulunmuyor.",
	}
	s.Archive = ArchiveStrings{
		Title:         "Arşivim",
		All:           "Tümü",
		SortRecent:    "Son eklenen",
		SortScore:     "Puan",
		SortName:      "Ad",
		EmptyTitle:    "Arşivin boş",
		EmptyBody:     "Bir şey ara, puan ver ya da durumunu işaretle. Burada toplanır.",
		GoSearch:      "Aramaya git",
		EmptyFiltered: "Bu filtrede içerik yok.",
	}
	s.Categories = CategoryStrings{
		Title:                  "Kategoriler",
		New:                    "Yeni kategori",
		Edit:                   "Kategoriyi düzenle",
		EmptyTitle:             "İlk kategorini oluştur",
		EmptyBody:              "Örneğin “Pazar akşamı kafa dağıtmalık” ya da “Twist'i olanlar”.",
		EmptyCategory:          "Bu kategori boş. Bir içeriğin detay sayfasından ekleyebilirsin.",
		Name:                   "Ad",
		NamePlaceholder:        "Pazar akşamı kafa dağıtmalık",
		Description:            "Açıklama",
		DescriptionPlaceholder: "İsteğe bağlı",
		Color:                  "Renk",
		Save:                   "Kaydet",
		Create:                 "Oluştur",
		Delete:                 "Kategoriyi sil",
		DeleteConfirm:          "Silmek için tekrar dokun",
		Remove:                 "Çıkar",
	}
	s.Errors = map[string]string{
		"score_out_of_range":            "Puan 1 ile 10 arasında olmalı.",
		"note_too_long":                 "Not en fazla 1000 karakter olabilir.",
		"category_name_required":        "Kategoriye bir ad ver.",
		"category_name_too_long":        "Ad en fazla 40 karakter olabilir.",
		"category_name_taken":           "Bu adda bir kategorin zaten var.",
		"category_description_too_long": "Açıklama en fazla 200 karakter olabilir.",
		"category_not_found":            "Kategori bulunamadı.",
		"title_not_found":               "İçerik bulunamadı.",
		"generic":                       "Bir şeyler ters gitti. Tekrar dene.",
		// web-only
		"invalid_color": "Listeden bir renk seç.",
	}
	s.About = AboutStrings{
		Title:              "Hakkında",
		Body:               "Kişisel film, dizi ve anime arşivin. Puanların ve kategorilerin sadece bu cihazda saklanır.",
		Sources:            "Veri kaynakları",
		TMDBAttribution:    "Film ve dizi verileri TMDB'den alınır. Bu ürün TMDB API'sini kullanır ancak TMDB tarafından onaylanmamış veya sertifikalandırılmamıştır.",
		AniListAttribution: "Anime verileri AniList'ten alınır.",
		TMDBStatusOn:       "TMDB bağlı",
		TMDBStatusOff:      "TMDB anahtarı yok: film ve dizi araması kapalı.",
	}
	s.Web = WebStrings{
		AppName:           "Film Arşivi",
		About:             "Hakkında",
		Theme:             "Tema",
		ThemeSystem:       "Sistem",
		ThemeLight:        "Açık",
		ThemeDark:         "Koyu",
		SearchButton:      "Ara",
		Loading:           "Yükleniyor…",
		StatusNone:        "Belirtilmedi",
		Save:              "Kaydet",
		Saved:             "Kaydedildi",
		NotFoundTitle:     "Sayfa bulunamadı",
		NotFoundBody:      "Aradığın şey burada değil. Aramadan yeniden bulabilirsin.",
		ServerError:       "Bir şeyler ters gitti. Tekrar dene.",
		Back:              "Geri",
		DeleteTitle:       "Kategori silinsin mi?",
		DeleteBody:        "Kategori ve içindeki liste silinir. İçeriklere verdiğin puanlar ve notlar kalır.",
		DeleteYes:         "Evet, sil",
		Cancel:            "Vazgeç",
		Sort:              "Sırala",
		Filter:            "Durum",
		Score:             "Puan",
		NoScore:           "Puan yok",
		CategoriesSaved:   "Kategoriler güncellendi",
		CategoryNamedHelp: "En fazla 40 karakter.",
		// The shared About.Body talks about "this device" (mobile). On the web the
		// data lives on the server, so the web page uses this text instead.
		AboutStorage:      "Kişisel film, dizi ve anime arşivin. Herkes kendi puanını verir, ortak puan yok. Puanların ve kategorilerin bu sunucuda saklanır.",
		OpenDetail:        "Detayı aç",
		Year:              "Yıl",
		Genres:            "Tür",
		Format:            "Biçim",
		SkipToContent:     "İçeriğe geç",
		CreateAndAdd:      "Oluştur ve bu içeriği ekle",
		Community:         "Topluluk",
		Votes:             "oy",
		YourScoreShort:    "Senin puanın",
		RateIt:            "Puan ver",
		IMDbAttribution:   "IMDb puanları IMDb'nin kişisel ve ticari olmayan kullanım için yayınladığı veri setlerinden alınır.",
		IMDbAttributionEN: "Information courtesy of IMDb (https://www.imdb.com). Used with permission.",
		Colors: map[string]string{
			"violet": "Mor", "blue": "Mavi", "teal": "Camgöbeği", "green": "Yeşil", "amber": "Kehribar",
			"orange": "Turuncu", "red": "Kırmızı", "pink": "Pembe", "slate": "Gri",
		},
	}
	return s
}()

// Error returns the message for a domain error code, or the generic message.
func (s Strings) Error(code string) string {
	if m, ok := s.Errors[code]; ok {
		return m
	}
	return s.Errors["generic"]
}
