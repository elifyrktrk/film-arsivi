# Film Arşivi — web (Go + sunucuda üretilen HTML)

Kişisel film, dizi ve anime arşivinin tek uygulaması: hem web sitesi hem de iPhone'da "Ana Ekrana Ekle" ile açılan mobil uygulama (PWA). Ayrı bir Expo uygulaması yok. Herkes kendi puanını verir (ortak puan yok), kendi kategorilerini oluşturur. Film ve diziler TMDB'den, animeler AniList'ten gelir.

Monorepo içinde kendi başına bir Go modülüdür (`filmarsivi/web`); pnpm/Turborepo ona dokunmaz.

## Kurulum

```bash
brew install go          # Go 1.25 veya üstü
cd apps/web
cp .env.example .env     # isteğe bağlı: TMDB_API_KEY ekle
go run ./cmd/web         # açılışta Mac'teki ve iPhone'daki adresleri yazdırır
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
| `ADDR` | `0.0.0.0:8090` | Dinlenecek adres. Varsayılan, aynı Wi-Fi'deki telefonların erişmesine izin verir; sadece bu Mac için `127.0.0.1:8090` ver. |
| `PORT` | `8090` | `ADDR` yoksa kullanılır. (8080 bu Mac'te Xcode simülatör aracı tarafından kullanıldığı için varsayılan 8090.) |
| `DB_PATH` | `data/film-arsivi.db` | SQLite dosyası. `data/` git'e girmez. |
| `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` | boş | Google ile giriş. İkisi de boşsa uygulama girişsiz, tek kullanıcılı çalışır (bkz. "Google ile giriş"). |
| `BASE_URL` | `http://localhost:<PORT>` | Sitenin dışarıdan görünen adresi. Redirect URI `BASE_URL/auth/google/callback` olur; `https://` ile başlıyorsa çerezler `Secure` olur. |
| `ALLOWED_EMAILS` | boş | Virgülle ayrılmış davet listesi. **Boşsa doğrulanmış e-postası olan her Google hesabı girebilir.** |
| `OWNER_EMAIL` | boş | Girişten önce oluşturulmuş puan ve kategoriler, bu adresle ilk giriş yapan kişiye aktarılır. |
| `IMDB_SYNC` | `on` | `off` verilirse IMDb veri dosyası indirilmez. Dosya zaten sadece `TMDB_API_KEY` varken indirilir. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error`. |

`.env` dosyası (varsa) açılışta okunur; ortamda zaten tanımlı olan değişkenleri ezmez. `.env` asla commit edilmez.

## iPhone'da deneme

1. Mac ve iPhone aynı Wi-Fi'de olsun.
2. Mac'te `go run ./cmd/web` çalıştır. Terminal şöyle bir satır yazar:
   `iPhone'dan:     http://192.168.x.x:8090   (aynı Wi-Fi)`
3. iPhone'da Safari'yi aç, bu adrese git.
4. Paylaş düğmesi → **Ana Ekrana Ekle**. Uygulama kendi simgesiyle, adres çubuğu olmadan tam ekran açılır.

macOS ilk açılışta gelen bağlantılara izin isteyebilir; "İzin Ver" de.

Bilmen gerekenler:

- **Mac kapalıyken ya da uykudayken çalışmaz.** Uygulama Mac'teki sunucuya bağlanır; veri de Mac'teki SQLite dosyasındadır. Kalıcı kullanım için sunucunun bir barındırma hizmetine (Fly.io, Render gibi, kalıcı diskle) yayınlanması gerekir. Bu sonraki adım.
- **Google girişi LAN adresinde çalışmaz.** Google, redirect URI olarak `192.168.x.x` gibi özel IP'leri kabul etmez; sadece `http://localhost` ya da HTTPS alan adı olabilir. Yani iPhone'dan LAN IP ile girince giriş açıksa oturum açılamaz. iPhone'da girişli kullanım için bir sunucuya yayınla (Fly.io) ya da HTTPS tünel kullan (ör. `cloudflared tunnel --url http://localhost:8090`, sonra `BASE_URL`'i tünel adresi yap ve o adresi Google'a ekle). Giriş kapalıyken (tek kullanıcılı mod) LAN adresi eskisi gibi çalışır, ama aynı Wi-Fi'deki herkes arşivi görüp değiştirebilir.
- Service worker yok: çevrimdışı çalışmaz. İkonlar ve manifest var, ana ekranda uygulama gibi açılır.

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
cmd/genicons       uygulama ikonlarını (apple-touch-icon, 192, 512 PNG) üretir
internal/domain    modeller, kurallar, puan ve kategori servisleri, arama birleştirme, Store arayüzü
internal/sources   TMDB ve AniList istemcileri
internal/auth      Google ile giriş: OIDC + PKCE akışı, oturumlar, davet listesi
internal/store     Store uygulamaları: Memory (referans) ve SQLite (modernc.org/sqlite, CGO yok)
internal/i18n      tüm Türkçe metinler tek dosyada (tr.go)
internal/web       handler'lar, html/template şablonları, CSS/JS (embed ile ikiliye gömülü)
```

Sadece standart kütüphane (`net/http`, `html/template`, `log/slog`) ve iki bağımlılık: `modernc.org/sqlite`, `golang.org/x/text` (Türkçe sıralama ve Unicode normalizasyonu). Framework yok, htmx yok.

### TypeScript çekirdeğiyle eşitlik

Expo uygulaması bırakıldığı için `packages/core` (TS) artık bir referans spesifikasyondur; davranışın asıl kaynağı Go tarafıdır (`internal/domain`, `internal/sources`). Yine de ikisi aynı kalsın diye:

- `internal/domain/domain_test.go`, `packages/core/src/core.test.ts` senaryolarını aynı adlar ve aynı verilerle birebir taşır. Store'a dokunan her senaryo hem Memory hem SQLite üzerinde çalışır.
- `internal/sources/sources_test.go`, AniList eşlemesini aynı Frieren örneğiyle doğrular; TMDB için v3/v4 anahtar ayrımını, Japon animasyonunun anime sayılmasını ve detay alanlarını sınar.
- Uzunluk sınırları JavaScript'teki gibi UTF-16 birimiyle sayılır, ad karşılaştırması `tr-TR` küçük harfle yapılır (İ/ı doğru), `Ad` sıralaması Türkçe harmanlamayla yapılır.
- `internal/i18n/i18n_test.go`, ortak metinlerin `packages/i18n/src/index.ts` içinde birebir durduğunu denetler.
- `internal/web/web_test.go` içindeki `TestTokensCopyIsFresh`, `tokens.css` kopyasının kaynakla aynı olduğunu denetler.

Çekirdekte bir kural değişirse: önce TS'de değiştir ve testini yaz, sonra aynı senaryoyu `domain_test.go`'ya ekle.

### Store ve kullanıcılar

`domain.Store` (`store.Store` takma adıyla da erişilir), `packages/core/src/repository.ts` sözleşmesinin Go hâlidir. Tek fark: kişiye ait her metot `userID` alır (`GetRating(ctx, userID, titleID)` gibi). `ratings` `(user_id, title_id)` ile, `categories` `user_id` ile ayrılır; `category_items` kategorisinin sahibi üzerinden süzülür (başka birinin kategori kimliğiyle yapılan okuma, ekleme, çıkarma, silme sessizce etkisizdir ya da 404 döner). `titles` ve `imdb_ratings` herkese ortak önbellektir. Giriş açıkken her istek oturumdaki kullanıcı adına, kapalıyken `web.LocalUser` (`"local"`) adına çalışır. Kategori adı benzersizliği kullanıcı başınadır.

## Google ile giriş

İsteğe bağlıdır. `GOOGLE_CLIENT_ID` ve `GOOGLE_CLIENT_SECRET` boşsa uygulama bugünkü gibi girişsiz ve tek kullanıcılı çalışır; Hakkında sayfası bunu söyler. İkisi de doluysa her sayfa giriş ister (Hakkında ve giriş sayfaları hariç).

### Google OAuth istemcisi oluşturma

1. [Google Cloud Console](https://console.cloud.google.com/) → yeni bir proje oluştur (ör. "Film Arşivi").
2. **APIs & Services → OAuth consent screen**: User type **External**. Uygulama adı, destek e-postası. Scopes: `openid`, `email`, `profile` (hassas olmayan kapsamlar, Google incelemesi gerekmez). Yayın durumu **Testing** kalabilir; **Test users** listesine kendi ve arkadaşlarının Gmail adreslerini ekle (Testing'de en fazla 100 test kullanıcısı girebilir).
3. **APIs & Services → Credentials → Create credentials → OAuth client ID**: Application type **Web application**.
   - **Authorized redirect URIs**: `http://localhost:8090/auth/google/callback` ve ileride yayın adresi, ör. `https://film-arsivi.fly.dev/auth/google/callback`.
   - Authorized JavaScript origins gerekmez (akış tamamen sunucu tarafında).
4. Çıkan Client ID ve Client secret'ı `.env`'e yaz:
   ```
   GOOGLE_CLIENT_ID=....apps.googleusercontent.com
   GOOGLE_CLIENT_SECRET=GOCSPX-...
   BASE_URL=http://localhost:8090
   ALLOWED_EMAILS=sen@gmail.com,arkadas@gmail.com
   OWNER_EMAIL=sen@gmail.com
   ```
5. `go run ./cmd/web`, sonra http://localhost:8090 → "Google ile oturum açın".

### Nasıl çalışıyor

- **Akış:** Sunucu tarafı OpenID Connect, Authorization Code + PKCE (S256) + `state` + `nonce`. `golang.org/x/oauth2` ve `github.com/coreos/go-oidc/v3` kullanılır; ID token'ın imzası, `iss`, `aud`, süresi ve `nonce`'u doğrulanır, `email_verified` şarttır. Kendi kripto kodu yok. Google keşfi (discovery) ilk girişte yapılır, böylece Google'a ulaşılamasa da uygulama açılır.
- **State:** Her giriş denemesi sunucuda `oauth_flows` tablosunda (state, nonce, PKCE verifier, dönüş adresi) 10 dakika tutulur ve tek kullanımlıktır. State ayrıca bu tarayıcıya `/auth/` yoluna sınırlı bir HttpOnly çerezle bağlanır; başka tarayıcıda başlatılmış bir girişin geri dönüşü reddedilir (login CSRF).
- **Oturum:** `sessions` tablosunda rastgele 256 bit token'ın SHA-256 özeti tutulur (veritabanı sızsa da oturum çalınamaz). Çerez `oturum`: HttpOnly, SameSite=Lax, `BASE_URL` HTTPS ise Secure, 30 gün. Süresi dolan oturum silinir ve giriş sayfasına yönlendirilir. "Çıkış yap" oturumu sunucuda da siler.
- **CSRF:** Tüm POST formları çift gönderimli (double-submit) bir token taşır (`csrf` çerezi + gizli alan, sabit zamanlı karşılaştırma). Önceki `Origin`/`Sec-Fetch-Site` kontrolü de duruyor.
- **Kullanıcılar:** `users` tablosu (`id`, `google_sub`, e-posta, ad, avatar). Kimlik Google'ın değişmeyen `sub` değeriyle eşleşir; e-posta değişirse güncellenir.
- **Davet listesi:** `ALLOWED_EMAILS` doluysa sadece bu adresler (büyük-küçük harf duyarsız) girebilir; diğerleri "Bu uygulama davetle çalışıyor" sayfasını görür ve oturum açılmaz. Boşsa her doğrulanmış Google hesabı girebilir; bu yüzden yayına almadan önce listeyi doldur.
- **Mevcut veri:** Giriş açılmadan önce oluşturulan puan ve kategoriler `local` kullanıcısına aittir. `OWNER_EMAIL` ile ilk giriş yapan kişiye tek transaction içinde aktarılır ve bu bir kez olur (`meta` tablosuna yazılır). Sahibin zaten aynı içerik için puanı varsa onunki korunur, eski kopya `local` altında kalır; hiçbir şey silinmez. `OWNER_EMAIL` boşsa eski veri `local` altında durur ve giriş kapatılınca yeniden görünür.

### Bilinen kısıtlar

- **LAN IP ile giriş olmaz.** Google redirect URI olarak sadece `http://localhost` ya da HTTPS alan adı kabul eder; `http://192.168.x.x` reddedilir. iPhone'da girişli kullanım için yayın (Fly.io, Render) ya da HTTPS tünel gerekir.
- **iOS ana ekran (PWA) ve OAuth: cihazda test edilmedi.** Ana ekrandan açılan uygulamada Google'a gidiş uygulama içi bir tarayıcı sayfasında açılır. iOS sürümüne göre bu sayfanın çerezleri ana ekran uygulamasıyla paylaşılmayabilir; o durumda giriş tamamlanır ama oturum çerezi uygulamaya geçmez ve kullanıcı yeniden giriş sayfasını görür. Önlem olarak state çerezi kaybolursa giriş güvenli biçimde reddedilir ("Giriş yarıda kaldı"), yanlış hesaba oturum açılmaz. Yayından sonra gerçek iPhone'da denenmeli; sorun çıkarsa çözüm, girişi Safari'de yapıp sonra ana ekrana eklemek ya da kısa ömürlü bir tek kullanımlık kodla oturumu uygulamaya aktarmak olur.
- Sahte bir OIDC sağlayıcıyla test edildi; gerçek Google istemcisiyle henüz denenmedi (anahtar yok).

## Liste ve ızgara

- **Arama sonuçları** kompakt liste satırı: solda 56×84 afiş (2:3, köşe 6), ad en fazla 2 satır, altında tek satır meta (TÜR · yıl · kaynak etiketi ya da topluluk rozeti), sağda varsa senin puanın. Satır ~100pt; 375pt ekranda arama kutusunun altında 6 sonuç tam görünür. Bütün satır tek dokunma hedefi. İsim hatırlamaya çalışırken taramak için liste ızgaradan daha rahat.
- **Arşivim ve kategori detayı** varsayılan olarak sıkı afiş ızgarası: telefonda 3 sütun (375pt'de afiş ~108×161pt, ad tek satır kırpılmış, puan afiş köşesinde; telefonda TÜR etiketi gizlenir, afiş zaten söylüyor). Masaüstünde sütun sayısı genişliğe göre artar (`auto-fill`, en az 140px; 1280px'te 7 sütun). Sağ üstteki ızgara/liste düğmesi `?gorunum=` ile değişir ve seçim `gorunum` çerezinde 1 yıl hatırlanır; liste görünümü arama satırıyla aynı bileşendir. Liste satırları en fazla 720px genişlikte ortalanır.
- Kategori detayında her öğenin "Çıkar" düğmesi satır bağlantısının dışında durur (iç içe dokunma hedefi yok).
- Statik dosyalar (`app.css`, `app.js`, `tokens.css`) içerik özetiyle sürümlenir (`?v=…`), yeni sürüm yayınlanınca tarayıcı eski önbelleği kullanmaz.
- Ölçüm (375×812, açık ve koyu tema): aramada 6 satır, Arşivim ızgarasında 6 afiş (2 sıra × 3), Arşivim listesinde 5 satır tam görünür.

## Arama: yarım yazılan adlar, yazarken arama, önbellek

- **Yarım kelimeler (Kitsu yedeği).** AniList'in araması kelime bazlıdır: `frie` hiçbir şey bulmaz, `frieren` bulur. Bu yüzden her anime aramasında AniList ile paralel olarak [Kitsu](https://kitsu.app/)'ya da sorulur (`filter[text]`, `include=mappings`, anahtarsız). AniList 5'ten az sonuç döndürürse Kitsu'nun bulduklarının AniList kimlikleri (`externalSite == "anilist/anime"`) alınır, eksik olanlar **tek bir** AniList `id_in` sorgusuyla çekilir ve AniList sonuçlarının arkasına eklenir. Kaynak etiketi AniList kalır; AniList eşlemesi olmayan Kitsu sonuçları atlanır. Arama başına en fazla 2 ek istek. Kitsu yanıt vermezse arama AniList sonuçlarıyla devam eder.
  - Doğrulandı: `fri`, `frie` → Frieren; `shinge` → Attack on Titan (Shingeki no Kyojin); `jujut` → Jujutsu Kaisen; `chainsa` → Chainsaw Man.
- **Yazarken arama.** Arama kutusunda 2. harften itibaren, son tuştan 300 ms sonra sadece sonuç alanı (`X-Partial: 1` ile sunucuda üretilen HTML parçası) yenilenir; önceki istek iptal edilir, adres çubuğundaki `?q=` güncellenir. JavaScript kapalıyken "Ara" butonu aynı sayfayı tam olarak yükler. htmx yerine 30 satırlık `static/app.js` yeterli oldu.
- **Önbellek.** Aynı arama (büyük-küçük harf ve boşluk fark etmeden) 10 dakika bellekte tutulur; hem anime (AniList + Kitsu) hem TMDB için. Hatalar önbelleğe alınmaz. AniList dakikada ~90 istek sınırı koyar; 429 gelirse arayüz "Biraz yavaşlayalım" diye sakin bir not gösterir (TMDB sonuçları varsa onlar yine listelenir).
- Hakkında sayfasında Kitsu atfı var.

## Topluluk puanları (IMDb ve AniList)

Kullanıcının kendi puanı her zaman önde ve büyük; topluluk puanı yanında, küçük ve ikincil durur. Puan yoksa hiçbir şey gösterilmez.

Görünüm:
- **Detay:** Solda senin puanın 56pt altın bir daire içinde ("9 · Senin puanın / 10"). Puan vermediysen kesik çizgili boş daire ve "Puan ver" bağlantısı çıkar, puan formuna kaydırır. Sağında ince bir ayraçla topluluk rozeti: kaynağın küçük işareti (IMDb sarısı, AniList laciverti) + değer, IMDb'de yanında soluk oy sayısı.
- **Kartlar:** En fazla bir küçük rozet, afişin üstünde değil metin satırında; kaynak etiketinin yerini alır. Senin puanın afişin köşesindeki altın rozet olarak kalır. Topluluk puanı yoksa sadece kaynak etiketi görünür.

- **Anime:** AniList'in `averageScore` değeri (0–100) 10 üzerinden tek ondalıkla gösterilir: 89 → "AniList 8,9". Anahtar gerekmez; arama sonuçlarıyla birlikte gelir.
- **Film ve dizi:** IMDb puanı, IMDb'nin kişisel ve ticari olmayan kullanım için yayınladığı [`title.ratings.tsv.gz`](https://datasets.imdbws.com/title.ratings.tsv.gz) dosyasından gelir (`tconst`, `averageRating`, `numVotes`). Gösterim: "IMDb 8,7 · 2,1 Mn oy". Oy sayısı Türkçe kısaltılır (850, 4,5 B, 45 B, 2,1 Mn).
  - Sunucu açılışta, kayıtlı kopya 23 saatten eskiyse dosyayı indirir, sonra saatte bir kontrol eder (yani günde bir yenilenir). İndirme `imdb_ratings` tablosuna tek transaction içinde tam yenileme olarak yazılır; indirme ya da ayrıştırma yarıda kalırsa eski veri olduğu gibi kalır. Ölçüm: ~1,7 milyon satır, bu Mac'te ~3,5 saniye.
  - Eşleştirme TMDB üzerinden yapılır: detay isteği `append_to_response=external_ids` ile `imdb_id`'yi alır ve `Title.IMDbID` olarak saklanır. TMDB anahtarı yoksa IMDb puanı sessizce gösterilmez ve dosya hiç indirilmez.
  - Arama kartlarında IMDb puanı görünmez, çünkü TMDB'nin arama yanıtında `imdb_id` yok ve her sonuç için ayrı istek atmak yavaş olur. Detay sayfasında, ve detayı açılmış (arşive ya da kategoriye eklenmiş) içeriklerin kartlarında görünür.
- Hakkında sayfasında IMDb atfı Türkçe ve IMDb'nin istediği İngilizce metinle yer alır.
- `Title`'a eklenen `imdbId` ve `anilistScore` alanları `packages/core`'da yok; Go tarafı artık asıl kaynak olduğu için orada bırakıldı. SQLite şeması eski veritabanlarına bu sütunları kendiliğinden ekler.
- Canlı indirme testi isteğe bağlı: `IMDB_LIVE=1 go test ./internal/store -run Live -v`.

## Verilen kararlar

- **Tek form, JS'siz çalışır.** Detay sayfasında puan, durum ve not tek "Kaydet" ile gönderilir (POST, sonra yönlendirme). Kategori üyeliği onay kutularıyla ayrı bir formdur. `static/app.js` yalnızca yükleme göstergesi ve çift gönderim engeli ekler; kapalıyken her şey çalışır.
- **htmx kullanılmadı.** İlk dilimde tam sayfa yenileme yeterince hızlı; bağımlılık eklemeye değmedi.
- **Silme onayı ayrı sayfa.** Mobildeki "tekrar dokun" JS gerektirir; web'de `/kategoriler/{id}/sil` onay sayfası var.
- **Tema.** `tokens.css` sistem temasını izler. Üst çubuktaki Sistem/Açık/Koyu düğmeleri `tema` çerezini yazar ve `<html data-theme>` ile tokens'ın kendi seçicilerini kullanır (JS gerekmez).
- **Mobil öncelikli.** Ana kullanım 375pt genişlik. Telefonda sekmeler (Ara, Arşivim, Kategoriler) alta sabitlenir; çentik ve home indicator için `env(safe-area-inset-*)` boşlukları var (`viewport-fit=cover`). Dokunma hedefleri en az 44pt; form alanları 16px+ yazıyla iOS'un otomatik yakınlaştırmasını tetiklemez. Masaüstünde aynı sayfalar genişler, sekmeler üst çubuğa çıkar.
- **PWA.** `static/manifest.webmanifest` (standalone, renkler `tokens.css`'ten: arka plan `--color-bg`, tema `--color-surface`), `apple-touch-icon.png` (180px), 192/512 ikonlar ve iOS meta etiketleri. Service worker eklenmedi (şart değildi; önbellek bayatlığı riskini prototipte almaya değmedi).
- **Tasarım dili.** Renk, boşluk ve köşe yarıçapları yalnızca `tokens.css` değişkenlerinden gelir; yazı ölçeği `packages/tokens` içindeki `type` ile aynıdır. Afişler her yerde 2:3.
- **Hakkında metni.** Ortak `about.body` "bu cihazda saklanır" diyor (mobil için doğru). Web'de veri sunucuda durduğu için `Web.AboutStorage` kullanılıyor.
- **Web'e özel metinler** `i18n.TR.Web` altında; ortak anahtarlara dokunulmadı. Geçersiz renk için web'e özel `invalid_color` hata kodu eklendi (TS'de tip sistemi bunu zaten engelliyor).
- **Kaynak çökerse.** Arama, bir kaynak yanıt vermezse diğerinin sonuçlarını gösterir ve not düşer; ikisi de yoksa "Tekrar dene" çıkar (502). Detay sayfası kaynak çökünce arşivdeki kayıtlı kopyayı "yüklenemedi" notuyla gösterir. Kaynağın detay yanıtları 15 dakika bellekte tutulur, böylece detay sayfası ve ardından gelen form aynı isteği tekrar atmaz. Arama sonuçları bu önbelleğe konmaz; onlarda sezon sayısı ve `imdb_id` gibi detay alanları eksiktir. Arşivdeki bir başlık açıldığında kayıtlı kopya tazelenir.
- **Güvenlik.** Giriş olmasa da başka kökenlerden gelen POST'lar reddedilir (`Origin`/`Sec-Fetch-Site`), sıkı CSP ve temel güvenlik başlıkları gönderilir, `geri` parametresi yalnızca site içi yollara yönlendirir. Sunucu telefondan erişilebilsin diye varsayılan olarak `0.0.0.0` üzerinde dinler (bkz. "iPhone'da deneme").
- **TMDB atfı.** Hakkında sayfasında TMDB'nin istediği metin hem Türkçe hem orijinal İngilizce hâliyle yer alır. TMDB'nin resmi logosu dosya olarak eklenmedi; yerine metin rozeti var.

## Uzun vade: yayınlama ve veri (karar bekliyor, uygulanmadı)

Giriş artık var (Google, sunucu tarafı oturum). Kalan soru verinin nerede duracağı:

Mobil artık bu uygulamanın kendisi olduğu için senkron sorunu kalmadı; asıl soru verinin nerede duracağı ve girişin nasıl yapılacağı. İki yol var:

1. **Go sunucusu + SQLite, kendi barındırmamızda.** Fly.io ya da Render'a kalıcı diskle yayınlanır; giriş zaten hazır. İleride başka istemciler gerekirse aynı servisler `/api/...` altında JSON olarak da sunulabilir; modeller zaten JSON etiketli. Artısı: tek parça, ucuz, veri bizde. Eksisi: giriş ve yedekleme bize kalır.
2. **Supabase.** Postgres + Auth + Row Level Security (`user_id = auth.uid()`). Bu Go sunucusu `domain.Store`'un Postgres uygulamasıyla Supabase veritabanını kullanır, girişi Supabase Auth yapar. Artısı: giriş, e-posta, yedek hazır. Eksisi: üçüncü taraf bağımlılığı, kuralların bir kısmı (benzersiz ad gibi) veritabanı kısıtı olarak da yazılmalı.

Arkadaşlarla küçük bir kullanım için 1. seçenek daha az parçalı; giriş ve çok cihazlı senkron hızlıca gerekirse 2. seçenek daha az iş. Store arayüzü ikisine de hazır.

## Sonraya bırakılanlar

- Bir sunucuya yayınlama (Fly.io + kalıcı disk) ve gerçek Google istemcisiyle giriş denemesi.
- iOS ana ekran uygulamasında girişin cihazda denenmesi.
- Service worker ile çevrimdışı açılış.
- Kategori içi elle sıralama (pozisyon alanı var, arayüz yok).
- Arama sonuçlarında sayfalama; TMDB ilk sayfa, AniList ilk 15 sonuç.
- Elle içerik ekleme (`manual` kaynağı modelde var).
- Arayüzün İngilizce çevirisi (`i18n` yapısı hazır).
