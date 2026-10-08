# olspanel

Lekki panel hostingowy w stylu DirectAdmin oparty o **OpenLiteSpeed** i **LSPHP**.
Jedna binarka Go, wbudowany interfejs React, instalacja jednym skryptem na
**Ubuntu 24.04 / 26.04**.

## Funkcje (MVP)

| Obszar | Co potrafi |
|---|---|
| Administrator | pakiety z limitami, użytkownicy, zawieszanie, „zaloguj jako”, status usług, restart OLS/MariaDB/FTP, dziennik zdarzeń |
| Domeny | domeny, subdomeny, aliasy; vhosty OLS generowane automatycznie; PHP 8.1–8.5 per domena; PHP działa jako użytkownik (LSAPI `extUser`) |
| SSL | Let's Encrypt (HTTP-01, biblioteka lego), auto-odnawianie, wymuszanie HTTPS |
| Bazy danych | MariaDB: bazy i użytkownicy `<user>_<nazwa>`, phpMyAdmin pod `/phpmyadmin/` |
| Pliki | menedżer plików w przeglądarce (upload drag&drop, edytor z podświetlaniem, zip/unzip, chmod), jail `os.Root` |
| FTP | Pure-FTPd z wirtualnymi użytkownikami i TLS |
| Cron | zadania cron per konto, crontab generowany z panelu |

Poza MVP (plan): DNS, e-mail, backupy, limity transferu, 2FA, kwoty dyskowe jądra.

## Instalacja

Na czystym serwerze Ubuntu 24.04 lub 26.04 (root):

```bash
git clone https://github.com/sikorafranciszek/olspanel.git
cd olspanel
sudo bash install.sh --from-source --hostname panel.example.com --email admin@example.com
```

Lub z gotowej binarki z GitHub Releases (po opublikowaniu wydania):

```bash
curl -fsSL https://raw.githubusercontent.com/sikorafranciszek/olspanel/main/install.sh -o install.sh
sudo bash install.sh --hostname panel.example.com --email admin@example.com
```

Opcje: `--admin-password`, `--php-versions "81 82 83 84"`, `--version vX.Y.Z`, `--skip-ftp`, `--enable-ufw`, `--http-port 80`, `--https-port 443`.
Instalator odmawia pracy, gdy porty 80/443/2222 zajmuje inny proces (podaj inne porty lub zatrzymaj usługę).

Po instalacji panel działa pod `https://<ip>:2222/` (login `admin`, hasło wypisane na końcu i zapisane w `/root/.olspanel_credentials`).
Instalator jest idempotentny: ponowne uruchomienie aktualizuje binarkę i migracje.

### Co robi instalator

1. Sprawdza Ubuntu 24.04/26.04 i architekturę (amd64/arm64).
2. Dodaje repozytorium LiteSpeed (klucz w `/usr/share/keyrings`, `signed-by`; fallback do `noble`, gdy brak pakietów dla danej wersji).
3. Instaluje `openlitespeed`, `lsphpXX` z rozszerzeniami, MariaDB, Pure-FTPd.
4. Pobiera phpMyAdmin (weryfikacja SHA-256) do `/usr/local/olspanel/phpmyadmin`.
5. Podmienia `httpd_config.conf` OLS na wersję z `include $SERVER_ROOT/conf/olspanel/*.conf` i wyłącza WebAdmin (nie rozumie `include`).
6. Tworzy bazę panelu, admina, domyślny pakiet, self-signed cert, unit systemd `olspanel`.

## Architektura

```
https://host:2222 ─► olspanel (Go, root, systemd)
                      ├─ /            SPA React (go:embed)
                      ├─ /api/v1/*    REST, sesje cookie + CSRF
                      ├─ /phpmyadmin/ proxy ► OLS vhost 127.0.0.1:8081
                      ├─ SQLite       /var/lib/olspanel/panel.db
                      └─ scheduler    odnawianie SSL, zużycie dysku
```

Panel jest jedynym źródłem prawdy dla konfiguracji OLS. Renderuje:

- `conf/olspanel/00-extprocessors.conf` – jeden `extprocessor` na parę konto × wersja PHP,
- `conf/olspanel/10-vhosts.conf` – bloki `virtualhost`,
- `conf/olspanel/20-listeners.conf` – listenery `:80`, `:443` (SNI, cert domeny lub panelu) i `127.0.0.1:8081` (phpMyAdmin),
- `conf/vhosts/<domena>/vhconf.conf` – docRoot, logi, `scripthandler`, `phpIniOverride` (`open_basedir`), `vhssl`, kontekst ACME.

Każda zmiana: render → zapis atomowy → `openlitespeed -t` → przy błędzie rollback → `lswsctrl restart` → sprawdzenie portu 80.

Układ konta: `/home/<user>/domains/<domena>/{public_html,logs,tmp}`. Katalog domowy ma `0750` + ACL `u:nobody:x`, więc inne konta nie czytają cudzych plików, a OLS (nobody) serwuje statyki. PHP działa jako użytkownik.

Certyfikaty: `/var/lib/olspanel/ssl/<domena>/`, konto ACME: `/var/lib/olspanel/acme/`. Challenge HTTP-01 jest serwowany z katalogu panelu (mapowanego kontekstem OLS), więc `.htaccess` użytkownika go nie zepsuje.

## Rozwój

Wymagania: Go ≥ 1.26, Node 22.

```bash
# backend w trybie deweloperskim (bez operacji systemowych, OLS dry-run)
make dev
# frontend z hot-reload (proxy /api -> :2222)
cd web && npm install && npm run dev
# testy
make test
# binarki linux/amd64 + arm64 i sumy kontrolne
make linux checksums
```

Zmienne środowiskowe (`/etc/olspanel/olspanel.env`): `OLSPANEL_LISTEN`, `OLSPANEL_HTTP_PORT`, `OLSPANEL_HTTPS_PORT`, `OLSPANEL_INSECURE_HTTP`, `OLSPANEL_DATA_DIR`, `OLSPANEL_LSWS_ROOT`, `OLSPANEL_HOME_ROOT`, `OLSPANEL_MYSQL_SOCKET`, `OLSPANEL_PMA_UPSTREAM`, `OLSPANEL_UPLOAD_MAX_MB`, `OLSPANEL_DEV_SPA`, `OLSPANEL_DEBUG`.

### Test end-to-end instalatora (WSL2)

```bash
bash scripts/test-install-wsl.sh            # Ubuntu 24.04
UBUNTU=26.04 bash scripts/test-install-wsl.sh
```

Skrypt importuje świeży rootfs Ubuntu jako osobną dystrybucję WSL (`olspanel-test-2404`), uruchamia `install.sh --from-source --http-port 18080 --https-port 18443` (dystrybucje WSL2 dzielą stos sieciowy, więc porty 80/443/3306 mogą być zajęte przez inne dystrybucje lub Docker; MariaDB dostaje `skip-networking`), a następnie `scripts/smoke.sh` tworzy przez API pakiet, użytkownika, domenę, bazę, konto FTP i cron, sprawdza je na poziomie systemu (PHP działa jako użytkownik, `openlitespeed -t`, `mysql`, `pure-pw`, `crontab -l`) i usuwa konto.

## CLI

```
olspanel serve                                  serwer (systemd)
olspanel init --admin-password ... [--hostname] [--email]
olspanel admin-reset-password [--password ...]  reset hasła admina
olspanel doctor                                 kontrola wymagań na hoście
olspanel version
```

## Bezpieczeństwo

- Proces działa jako root (jak DirectAdmin). Operacje uprzywilejowane są w jednym pakiecie `internal/system` z allowlistą binarek; nigdy nie jest wywoływany shell.
- Każda nazwa trafiająca do configu OLS, crontaba, `pure-pw` lub SQL przechodzi przez `internal/validate` (regexy + IDNA), a render OLS waliduje wartości ponownie.
- Menedżer plików używa `os.Root` – brak możliwości wyjścia z `/home/<user>` przez `..` ani symlinki.
- Sesje: argon2id, cookie HttpOnly/Secure/SameSite=Strict, token CSRF, limit prób logowania, dziennik zdarzeń.
- Limit ~100 `extprocessor` w OLS = ok. 100 par konto × wersja PHP na serwer.

## Licencja

MIT
