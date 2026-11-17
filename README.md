# Film Arşivi

Kişisel film, dizi ve anime arşivi. Herkes kendi puanını verir (ortak puan yok) ve kendi kategorilerini oluşturur. İçerik TMDB'den (film ve dizi) ve AniList'ten (anime) gelir.

## Yapı

| Klasör | Ne |
|---|---|
| `apps/web` | Asıl uygulama: Go ve sunucuda üretilen HTML. Hem web sitesi hem iPhone'daki uygulama (Ana Ekrana Ekle). Kurulum ve çalıştırma `apps/web/README.md` içinde. |
| `packages/tokens` | Renk, boşluk, köşe ve yazı token'ları, açık ve koyu tema. `pnpm --filter @film-arsivi/tokens build:css` ile `dist/tokens.css` ve `dist/tokens.json` üretilir; Go uygulaması bunları kullanır. |
| `packages/i18n` | Türkçe arayüz metinlerinin TypeScript kaynağı. |
| `packages/core` | Alan modelinin TypeScript referans uygulaması ve testleri (puan, kategori, arama birleştirme). Şu an bir tüketicisi yok; Go tarafı davranışın asıl kaynağı. |

## Kararlar

- **Tek uygulama, Go ve HTML (17 Kasım 2026).** İlk plan React Native/Expo mobil uygulama ve ayrıca bir web sitesiydi. Kullanıcı Go ve HTML ile tek uygulamayı seçti. iPhone'da Safari'den "Ana Ekrana Ekle" ile tam ekran açılır (PWA). Artısı: tek kod tabanı ve hızlı prototip. Eksisi: App Store'da yok, çevrimdışı kullanım ve bildirimler sınırlı. Gerçek bir iOS uygulaması gerekirse aynı Go sunucusunun JSON API'si üzerine yazılabilir.
- **İlk sürüm tek kullanıcılı, giriş yok.** Önce kullanıcı ve arkadaşları kullanacak. Depolama arayüzü ileride kullanıcı kimliği alacak biçimde tasarlandı.
- **MVP'de tek puan (1–10)**, kriter bazlı puan sonraki aşamada. **Kategoriler elle doldurulur**, akıllı kurallar sonraki aşamada.
- **İçerik kaynakları:** AniList anahtarsız çalışır. TMDB için `TMDB_API_KEY` gerekir; yoksa uygulama sadece anime arar ve bunu belirtir. Aynı anime iki kaynakta da çıkarsa AniList kaydı tutulur.
- **Lisans:** TMDB ve AniList ticari olmayan kullanım için ücretsiz. TMDB atfı Hakkında sayfasında.

## Komutlar

```bash
pnpm install
pnpm test        # packages/core testleri
pnpm typecheck
```
