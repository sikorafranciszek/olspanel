# REST API olspanel

Baza: `https://<host>:2222/api/v1`. Odpowiedzi JSON; błędy mają postać `{"error": "...", "code": "..."}`.

## Uwierzytelnianie

- Sesja w cookie `olspanel_session` (HttpOnly, Secure, SameSite=Strict).
- Każde żądanie zmieniające stan (POST/PUT/DELETE) wymaga nagłówka `X-CSRF-Token` z wartością `csrf` zwróconą przez `POST /auth/login` lub `GET /auth/me`.
- Role: `admin` (trasy `/admin/*`) i `user`. Admin może „zalogować się jako” użytkownik (`impersonate`) – wtedy działa z jego uprawnieniami, a dziennik zapisuje oba identyfikatory.

| Metoda | Ścieżka | Opis |
|---|---|---|
| GET | `/health` | `{ok, version}` bez logowania |
| POST | `/auth/login` | `{username, password}` → `{user, csrf, version}` (limit 5/min na IP) |
| POST | `/auth/logout` | |
| GET | `/auth/me` | `{user, impersonating, csrf, version}` |
| PUT | `/auth/password` | `{current, new}` |
| POST | `/auth/stop-impersonation` | wraca do konta admina |
| GET | `/usage` | zużycie vs limity pakietu |
| GET | `/php/versions` | wersje PHP dostępne dla konta (zainstalowane ∩ pakiet) |

## Użytkownik

### Domeny
| Metoda | Ścieżka | Body |
|---|---|---|
| GET | `/domains` | |
| POST | `/domains` | `{name, type: "domain"\|"subdomain"\|"alias", parent_id?, php_version?}` |
| PUT | `/domains/{id}` | `{php_version?, force_https?}` |
| DELETE | `/domains/{id}` | usuwa też subdomeny/aliasy i katalog |
| POST | `/domains/{id}/ssl/issue` | wystawia cert Let's Encrypt (domena + www + aliasy; fallback do samej domeny) |

### Bazy danych
| Metoda | Ścieżka | Body |
|---|---|---|
| GET | `/databases` | bazy z listą użytkowników |
| POST | `/databases` | `{suffix}` → baza `<user>_<suffix>` |
| DELETE | `/databases/{id}` | |
| POST | `/databases/{id}/users` | `{suffix, password}` |
| PUT | `/databases/{id}/users/{uid}` | `{password}` |
| DELETE | `/databases/{id}/users/{uid}` | |

### Pliki (ścieżki względem `/home/<user>`)
| Metoda | Ścieżka | Parametry |
|---|---|---|
| GET | `/files?path=` | lista katalogu |
| GET | `/files/content?path=` | `{content}` (≤ 2 MB) |
| PUT | `/files/content` | `{path, content}` |
| GET | `/files/download?path=` | plik |
| POST | `/files/upload?path=` | multipart `files[]` |
| POST | `/files/mkdir` | `{path}` |
| POST | `/files/rename` | `{from, to}` |
| POST | `/files/copy` | `{from, to}` |
| POST | `/files/delete` | `{paths: []}` |
| POST | `/files/chmod` | `{path, mode: "0644"}` |
| POST | `/files/compress` | `{paths: [], dest: "x.zip"}` |
| POST | `/files/extract` | `{path: "x.zip", dest: "dir"}` |

### FTP
| Metoda | Ścieżka | Body |
|---|---|---|
| GET | `/ftp` | |
| POST | `/ftp` | `{suffix, password, home_subdir}` (`suffix` pusty = login `<user>`) |
| PUT | `/ftp/{id}` | `{password?, home_subdir?}` |
| DELETE | `/ftp/{id}` | |

### Cron
| Metoda | Ścieżka | Body |
|---|---|---|
| GET | `/cron` | |
| POST | `/cron` | `{schedule, command, enabled}` |
| PUT | `/cron/{id}` | `{schedule, command, enabled}` |
| DELETE | `/cron/{id}` | |

## Administrator (`/admin`)

| Metoda | Ścieżka | Body / opis |
|---|---|---|
| GET | `/admin/server` | hostname, wersje, zasoby, usługi, PHP |
| POST | `/admin/services/{lsws\|mariadb\|pure-ftpd\|cron}/restart` | |
| POST | `/admin/ols/apply` | wymusza re-render konfiguracji OLS |
| GET/PUT | `/admin/settings` | `panel_hostname`, `acme_email`, `acme_staging` (`"0"/"1"`), `default_php`, `ftp_passive_ip` |
| GET | `/admin/audit` | ostatnie 200 wpisów |
| GET/POST | `/admin/users` | `{username, password, email, role, package_id}` |
| PUT | `/admin/users/{id}` | `{email, package_id, password?}` |
| DELETE | `/admin/users/{id}` | kaskadowo usuwa wszystko |
| POST | `/admin/users/{id}/suspend` / `unsuspend` | |
| POST | `/admin/users/{id}/impersonate` | → `{user, impersonating: true, csrf}` |
| GET/POST | `/admin/packages` | `{name, disk_mb, max_domains, max_subdomains, max_databases, max_ftp, max_cron, php_versions: ["83"]}` |
| PUT/DELETE | `/admin/packages/{id}` | |
| GET | `/admin/domains` | wszystkie domeny |

## phpMyAdmin

`/phpmyadmin/` jest reverse-proxy do vhosta OLS na `127.0.0.1:8081`, dostępnym tylko dla zalogowanych użytkowników panelu. Logowanie w phpMyAdmin: dane użytkownika bazy (`<user>_<nazwa>`).
