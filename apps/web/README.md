# Film Arşivi — web (Go + sunucuda üretilen HTML)

Kişisel film, dizi ve anime arşivinin web sitesi. Herkes kendi puanını verir (ortak puan yok), kendi kategorilerini oluşturur. Film ve diziler TMDB'den, animeler AniList'ten gelir.

Monorepo içinde kendi başına bir Go modülüdür (`filmarsivi/web`); pnpm/Turborepo ona dokunmaz.

## Kurulum

```bash
brew install go          # Go 1.25 veya üstü
cd apps/web
cp .env.example .env     # isteğe bağlı: TMDB_API_KEY ekle
go run ./cmd/web         # http://127.0.0.1:8080
```

Testler ve denetim:

```bash
go test ./...
go vet ./...
```

`packages/tokens` değişirse tasarım belirteçlerini yeniden kopyala (aksi hâlde `go test` uyarır):

```bash
go generate ./...
```

## Ortam değişkenleri

| Değişken | Varsayılan | Açıklama |
| --- | --- | --- |
| `TMDB_API_KEY` | boş | TMDB v3 anahtarı ya da `eyJ` ile başlayan v4 okuma token'ı. Yoksa site sadece AniList ile anime arar ve arayüz bunu sakin bir notla söyler. |
| `ADDR` | `127.0.0.1:8080` | Dinlenecek adres. Sadece `PORT` vermek de olur. |
| `DB_PATH` | `data/film-arsivi.db` | SQLite dosyası. `data/` git'e girmez. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error`. |

`.env` dosyası (varsa) açılışta okunur; ortamda zaten tanımlı olan değişkenleri ezmez. `.env` asla commit edilmez.

## Sayfalar

| Yol | Sayfa |
| --- | --- |
| `/` , `/?q=…` | Ara: film, dizi ve anime tek listede, afişli, kaynak etiketli |
| `/arsiv?durum=…&sirala=…` | Arşivim: durum filtresi (Tümü, İzledim, İzleyeceğim, Yarıda bıraktım), sıralama (Son eklenen, Puan, Ad) |
| `/icerik/{id}` | Detay: özet, yıl, tür, sezon ve bölüm sayısı; 1–10 puan, durum, not; kategorilere ekle/çıkar |
| `/kategoriler`, `/kategoriler/yeni`, `/kategoriler/{id}`, `/kategoriler/{id}/duzenle`, `/kategoriler/{id}/sil` | Kategori listesi, oluştur, detay, düzenle, silme onayı |
| `/hakkinda` | Veri kaynakları ve TMDB atıf metni |

`{id}` çekirdekteki kimlik biçimidir: `tmdb:movie:603`, `tmdb:tv:1399`, `anilist:21`.

## Mimari

```
cmd/web            main: yapılandırma, SQLite, kaynak istemcileri, HTTP sunucusu
cmd/synctokens     packages/tokens/dist/tokens.css → internal/web/static/tokens.css
internal/domain    modeller, kurallar, puan ve kategori servisleri, arama birleştirme, Store arayüzü
internal/sources   TMDB ve AniList istemcileri
internal/store     Store uygulamaları: Memory (referans) ve SQLite (modernc.org/sqlite, CGO yok)
internal/i18n      tüm Türkçe metinler tek dosyada (tr.go)
internal/web       handler'lar, html/template şablonları, CSS/JS (embed ile ikiliye gömülü)
```

Sadece standart kütüphane (`net/http`, `html/template`, `log/slog`) ve iki bağımlılık: `modernc.org/sqlite`, `golang.org/x/text` (Türkçe sıralama ve Unicode normalizasyonu). Framework yok, htmx yok.

### TypeScript çekirdeğiyle eşitlik

`internal/domain` ve `internal/sources`, `packages/core`'un Go karşılığıdır. Davranışı aynı tutmak için:

- `internal/domain/domain_test.go`, `packages/core/src/core.test.ts` senaryolarını aynı adlar ve aynı verilerle birebir taşır. Store'a dokunan her senaryo hem Memory hem SQLite üzerinde çalışır.
- `internal/sources/sources_test.go`, AniList eşlemesini aynı Frieren örneğiyle doğrular; TMDB için v3/v4 anahtar ayrımını, Japon animasyonunun anime sayılmasını ve detay alanlarını sınar.
- Uzunluk sınırları JavaScript'teki gibi UTF-16 birimiyle sayılır, ad karşılaştırması `tr-TR` küçük harfle yapılır (İ/ı doğru), `Ad` sıralaması Türkçe harmanlamayla yapılır.
- `internal/i18n/i18n_test.go`, ortak metinlerin `packages/i18n/src/index.ts` içinde birebir durduğunu denetler.
- `internal/web/web_test.go` içindeki `TestTokensCopyIsFresh`, `tokens.css` kopyasının kaynakla aynı olduğunu denetler.

Çekirdekte bir kural değişirse: önce TS'de değiştir ve testini yaz, sonra aynı senaryoyu `domain_test.go`'ya ekle.

### Store ve kullanıcılar

`domain.Store` (`store.Store` takma adıyla da erişilir), `packages/core/src/repository.ts` sözleşmesinin Go hâlidir. Tek fark: kişiye ait her metot şimdiden `userID` alır (`GetRating(ctx, userID, titleID)` gibi). `titles` tablosu ortak bir önbellektir, kullanıcıya bağlı değildir. Şimdilik giriş yok ve tüm istekler `web.LocalUser` (`"local"`) adına çalışır. Hesaplar eklendiğinde bu sabit, oturumdaki kullanıcıyla değişir; şema ve servisler aynı kalır. `categories.user_id` ve `ratings(user_id, title_id)` bunun için var.

## Verilen kararlar

- **Tek form, JS'siz çalışır.** Detay sayfasında puan, durum ve not tek "Kaydet" ile gönderilir (POST, sonra yönlendirme). Kategori üyeliği onay kutularıyla ayrı bir formdur. `static/app.js` yalnızca yükleme göstergesi ve çift gönderim engeli ekler; kapalıyken her şey çalışır.
- **htmx kullanılmadı.** İlk dilimde tam sayfa yenileme yeterince hızlı; bağımlılık eklemeye değmedi.
- **Silme onayı ayrı sayfa.** Mobildeki "tekrar dokun" JS gerektirir; web'de `/kategoriler/{id}/sil` onay sayfası var.
- **Tema.** `tokens.css` sistem temasını izler. Üst çubuktaki Sistem/Açık/Koyu düğmeleri `tema` çerezini yazar ve `<html data-theme>` ile tokens'ın kendi seçicilerini kullanır (JS gerekmez).
- **Tasarım dili.** Renk, boşluk ve köşe yarıçapları yalnızca `tokens.css` değişkenlerinden gelir; yazı ölçeği `packages/tokens` içindeki `type` ile aynıdır. Afişler her yerde 2:3. Telefonda sekmeler mobil uygulamadaki gibi alt çubuğa iner.
- **Hakkında metni.** Ortak `about.body` "bu cihazda saklanır" diyor (mobil için doğru). Web'de veri sunucuda durduğu için `Web.AboutStorage` kullanılıyor.
- **Web'e özel metinler** `i18n.TR.Web` altında; ortak anahtarlara dokunulmadı. Geçersiz renk için web'e özel `invalid_color` hata kodu eklendi (TS'de tip sistemi bunu zaten engelliyor).
- **Kaynak çökerse.** Arama, bir kaynak yanıt vermezse diğerinin sonuçlarını gösterir ve not düşer; ikisi de yoksa "Tekrar dene" çıkar (502). Detay sayfası kaynak çökünce arşivdeki kayıtlı kopyayı "yüklenemedi" notuyla gösterir. Kaynaktan gelen başlıklar 15 dakika bellekte tutulur, böylece detay sayfası ve ardından gelen form aynı isteği tekrar atmaz. Arşivdeki bir başlık açıldığında kayıtlı kopya tazelenir.
- **Güvenlik.** Giriş olmasa da başka kökenlerden gelen POST'lar reddedilir (`Origin`/`Sec-Fetch-Site`), sıkı CSP ve temel güvenlik başlıkları gönderilir, `geri` parametresi yalnızca site içi yollara yönlendirir. Sunucu varsayılan olarak sadece `127.0.0.1` üzerinde dinler.
- **TMDB atfı.** Hakkında sayfasında TMDB'nin istediği metin hem Türkçe hem orijinal İngilizce hâliyle yer alır. TMDB'nin resmi logosu dosya olarak eklenmedi; yerine metin rozeti var.

## Uzun vade: mobil ile senkron (karar bekliyor, uygulanmadı)

Mobil uygulama bugün kendi SQLite'ında çalışıyor. Web ile aynı veriyi görmek için iki yol var:

1. **Bu Go sunucusu backend olur.** Aynı servisler üzerine `/api/...` altında bir JSON API (ör. `GET /api/archive`, `PUT /api/ratings/{titleId}`, `POST /api/categories`) eklenir; modeller zaten JSON etiketli ve TS tipleriyle aynı alan adlarını kullanıyor. Mobil, `Repositories` arayüzünün bir HTTP uygulamasını yazar ve çevrimdışı değişiklikleri `updatedAt` ile sıraya alıp gönderir. Artısı: tek kod tabanında kurallar, veri kendi sunucunda, ek maliyet yok. Eksisi: kimlik doğrulama, yedekleme, barındırma ve senkron çakışma kuralları bize kalır.
2. **Supabase.** Postgres + Auth + Row Level Security (`user_id = auth.uid()`), mobil ve web doğrudan Supabase'e bağlanır; `repository.ts` için bir Supabase uygulaması yazılır, bu Go sunucusu da `domain.Store`'un Postgres uygulamasıyla aynı veritabanını okur. Artısı: giriş, e-posta, yedek ve gerçek zamanlı abonelik hazır. Eksisi: üçüncü taraf bağımlılığı, kuralların bir kısmı (benzersiz ad gibi) veritabanı kısıtı olarak da yazılmalı.

Arkadaşlarla küçük bir kullanım için 1. seçenek daha az parçalı; giriş ve çok cihazlı senkron hızlıca gerekirse 2. seçenek daha az iş. Store arayüzü ikisine de hazır.

## Sonraya bırakılanlar

- Giriş ve çok kullanıcı (Store hazır, oturum yok).
- JSON API ve mobil senkron (yukarıdaki karar).
- Kategori içi elle sıralama (pozisyon alanı var, arayüz yok).
- Arama sonuçlarında sayfalama; TMDB ilk sayfa, AniList ilk 15 sonuç.
- Elle içerik ekleme (`manual` kaynağı modelde var).
- Arayüzün İngilizce çevirisi (`i18n` yapısı hazır).
