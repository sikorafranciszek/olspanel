#!/usr/bin/env bash
#
# olspanel installer for Ubuntu 24.04 / 26.04
#
#   curl -fsSL https://raw.githubusercontent.com/sikorafranciszek/olspanel/main/install.sh -o install.sh
#   sudo bash install.sh [--hostname panel.example.com] [--admin-password ...] [--php-versions "81 82 83 84"]
#                        [--from-source] [--version vX.Y.Z] [--skip-ftp] [--enable-ufw] [--email you@example.com]
#                        [--http-port 80] [--https-port 443]
#
set -euo pipefail

OLSPANEL_REPO="${OLSPANEL_REPO:-sikorafranciszek/olspanel}"
PANEL_DIR=/usr/local/olspanel
DATA_DIR=/var/lib/olspanel
LOG_DIR=/var/log/olspanel
LSWS=/usr/local/lsws
PMA_VERSION="${PMA_VERSION:-5.2.2}"
LOG=/var/log/olspanel-install.log
PANEL_PORT=2222

HOSTNAME_OPT=""
ADMIN_PASS=""
PHP_VERSIONS="81 82 83 84"
FROM_SOURCE=0
VERSION="latest"
SKIP_FTP=0
ENABLE_UFW=0
ACME_EMAIL=""
HTTP_PORT=80
HTTPS_PORT=443

usage() {
  sed -n '2,8p' "$0"
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --hostname) HOSTNAME_OPT="$2"; shift 2 ;;
    --admin-password) ADMIN_PASS="$2"; shift 2 ;;
    --php-versions) PHP_VERSIONS="$2"; shift 2 ;;
    --from-source) FROM_SOURCE=1; shift ;;
    --version) VERSION="$2"; shift 2 ;;
    --skip-ftp) SKIP_FTP=1; shift ;;
    --enable-ufw) ENABLE_UFW=1; shift ;;
    --email) ACME_EMAIL="$2"; shift 2 ;;
    --http-port) HTTP_PORT="$2"; shift 2 ;;
    --https-port) HTTPS_PORT="$2"; shift 2 ;;
    -h|--help) usage ;;
    *) echo "Nieznana opcja: $1"; usage ;;
  esac
done

# ---------------------------------------------------------------- helpers
mkdir -p "$(dirname "$LOG")"
exec > >(tee -a "$LOG") 2>&1

c_green='\033[0;32m'; c_yellow='\033[0;33m'; c_red='\033[0;31m'; c_off='\033[0m'
info() { echo -e "${c_green}[olspanel]${c_off} $*"; }
warn() { echo -e "${c_yellow}[olspanel]${c_off} $*"; }
die()  { echo -e "${c_red}[olspanel] BŁĄD:${c_off} $*" >&2; exit 1; }

export DEBIAN_FRONTEND=noninteractive
APT="apt-get -y -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold"

# ---------------------------------------------------------------- checks
[[ $EUID -eq 0 ]] || die "uruchom jako root (sudo bash install.sh)"
[[ -r /etc/os-release ]] || die "brak /etc/os-release"
# shellcheck disable=SC1091
. /etc/os-release
[[ "${ID:-}" == "ubuntu" ]] || die "obsługiwane jest tylko Ubuntu (wykryto: ${ID:-?})"
case "${VERSION_ID:-}" in
  24.04|26.04) ;;
  *) die "obsługiwane wersje: Ubuntu 24.04 i 26.04 (wykryto: ${VERSION_ID:-?})" ;;
esac
CODENAME="${VERSION_CODENAME:-}"
ARCH="$(dpkg --print-architecture)"
case "$ARCH" in amd64|arm64) ;; *) die "nieobsługiwana architektura: $ARCH" ;; esac
command -v systemctl >/dev/null || die "wymagany systemd"

info "Ubuntu $VERSION_ID ($CODENAME), $ARCH"

# An existing install keeps its ports (from the env file) unless overridden on the command line.
if [[ -f /etc/olspanel/olspanel.env ]]; then
  # shellcheck disable=SC1091
  . /etc/olspanel/olspanel.env
  [[ "$HTTP_PORT" == "80" ]] && HTTP_PORT="${OLSPANEL_HTTP_PORT:-80}"
  [[ "$HTTPS_PORT" == "443" ]] && HTTPS_PORT="${OLSPANEL_HTTPS_PORT:-443}"
fi

# Ports must be free (or already held by OpenLiteSpeed / the panel from a previous run).
port_owner() { # <port> -> process name or empty
  ss -Hltnp "sport = :$1" 2>/dev/null | sed -n 's/.*users:(("\([^"]*\)".*/\1/p' | head -1
}
for p in "$HTTP_PORT" "$HTTPS_PORT" "$PANEL_PORT"; do
  owner="$(port_owner "$p" || true)"
  case "$owner" in
    ""|openlitespeed|litespeed|lshttpd|olspanel) ;;
    *) die "port $p jest zajęty przez '$owner'. Zatrzymaj tę usługę albo użyj --http-port/--https-port." ;;
  esac
done
info "log instalacji: $LOG"

# ---------------------------------------------------------------- base packages
info "Instaluję pakiety bazowe"
$APT update
$APT install curl wget gnupg ca-certificates lsb-release unzip acl cron software-properties-common openssl python3

# ---------------------------------------------------------------- LiteSpeed repo
if [[ ! -f /etc/apt/sources.list.d/litespeed.list ]]; then
  info "Dodaję repozytorium LiteSpeed"
  curl -fsSL https://rpms.litespeedtech.com/debian/lst_debian_repo.gpg -o /tmp/lst_debian_repo.gpg
  curl -fsSL https://rpms.litespeedtech.com/debian/lst_repo.gpg -o /tmp/lst_repo.gpg
  install_key() { # <src> <dst>  (accepts armored or binary keys)
    if head -c 20 "$1" | grep -q "BEGIN PGP"; then gpg --dearmor < "$1" > "$2"; else cp "$1" "$2"; fi
    chmod 644 "$2"
  }
  install_key /tmp/lst_debian_repo.gpg /usr/share/keyrings/litespeed.gpg
  install_key /tmp/lst_repo.gpg /usr/share/keyrings/litespeed-repo.gpg
  rm -f /tmp/lst_debian_repo.gpg /tmp/lst_repo.gpg
  REPO_DIST="$CODENAME"
  if ! curl -fsSIL "https://rpms.litespeedtech.com/debian/dists/${CODENAME}/Release" >/dev/null 2>&1; then
    warn "repozytorium LiteSpeed nie ma jeszcze dystrybucji '${CODENAME}', używam 'noble'"
    REPO_DIST="noble"
  fi
  echo "deb [arch=${ARCH} signed-by=/usr/share/keyrings/litespeed.gpg,/usr/share/keyrings/litespeed-repo.gpg] https://rpms.litespeedtech.com/debian/ ${REPO_DIST} main" > /etc/apt/sources.list.d/litespeed.list
  $APT update
fi

# ---------------------------------------------------------------- OpenLiteSpeed + LSPHP
info "Instaluję OpenLiteSpeed"
$APT install openlitespeed

for v in $PHP_VERSIONS; do
  if apt-cache show "lsphp${v}" >/dev/null 2>&1; then
    info "Instaluję LSPHP $v"
    pkgs="lsphp${v} lsphp${v}-common lsphp${v}-mysql"
    for ext in curl intl imagick opcache redis memcached imap igbinary msgpack; do
      if apt-cache show "lsphp${v}-${ext}" >/dev/null 2>&1; then pkgs="$pkgs lsphp${v}-${ext}"; fi
    done
    # shellcheck disable=SC2086
    $APT install $pkgs || warn "nie udało się zainstalować wszystkich rozszerzeń dla lsphp${v}"
  else
    warn "pakiet lsphp${v} nie jest dostępny w repozytorium, pomijam"
  fi
done
ls -d ${LSWS}/lsphp?? >/dev/null 2>&1 || die "nie zainstalowano żadnej wersji LSPHP"
NEWEST_PHP="$(ls -d ${LSWS}/lsphp?? | sort | tail -1 | sed 's#.*/lsphp##')"

# ---------------------------------------------------------------- MariaDB
info "Instaluję MariaDB"
$APT install mariadb-server mariadb-client
systemctl enable --now mariadb
# root uses unix_socket auth by default on Ubuntu; make sure.
mysql -e "ALTER USER 'root'@'localhost' IDENTIFIED VIA unix_socket; FLUSH PRIVILEGES;" 2>/dev/null || true
mysql -e "DELETE FROM mysql.user WHERE User=''; DROP DATABASE IF EXISTS test; FLUSH PRIVILEGES;" 2>/dev/null || true

# ---------------------------------------------------------------- pure-ftpd
if [[ $SKIP_FTP -eq 0 ]]; then
  info "Instaluję Pure-FTPd"
  $APT install pure-ftpd pure-ftpd-common
  mkdir -p /etc/pure-ftpd/conf /etc/pure-ftpd/auth
  sed -i 's/^STANDALONE_OR_INETD=.*/STANDALONE_OR_INETD=standalone/' /etc/default/pure-ftpd-common
  sed -i 's/^VIRTUALCHROOT=.*/VIRTUALCHROOT=false/' /etc/default/pure-ftpd-common
  echo "/etc/pure-ftpd/pureftpd.pdb" > /etc/pure-ftpd/conf/PureDB
  echo "no"   > /etc/pure-ftpd/conf/PAMAuthentication
  echo "no"   > /etc/pure-ftpd/conf/UnixAuthentication
  echo "yes"  > /etc/pure-ftpd/conf/ChrootEveryone
  echo "yes"  > /etc/pure-ftpd/conf/NoAnonymous
  echo "1000" > /etc/pure-ftpd/conf/MinUID
  echo "30000 30100" > /etc/pure-ftpd/conf/PassivePortRange
  echo "1"    > /etc/pure-ftpd/conf/TLS
  echo "133 022" > /etc/pure-ftpd/conf/Umask
  echo "yes"  > /etc/pure-ftpd/conf/DontResolve
  echo "yes"  > /etc/pure-ftpd/conf/CreateHomeDir
  rm -f /etc/pure-ftpd/auth/70pam /etc/pure-ftpd/auth/65unix
  ln -sf ../conf/PureDB /etc/pure-ftpd/auth/50pure
  touch /etc/pure-ftpd/pureftpd.passwd
  pure-pw mkdb /etc/pure-ftpd/pureftpd.pdb -f /etc/pure-ftpd/pureftpd.passwd
fi

# ---------------------------------------------------------------- panel binary
info "Instaluję olspanel"
mkdir -p "$PANEL_DIR/bin" "$PANEL_DIR/share" "$DATA_DIR" "$LOG_DIR" /etc/olspanel
chmod 700 "$DATA_DIR"

SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ $FROM_SOURCE -eq 1 ]]; then
  [[ -f "$SRC_DIR/go.mod" ]] || die "--from-source wymaga uruchomienia z katalogu repozytorium"
  if ! command -v go >/dev/null || ! go version | grep -qE 'go1\.(2[6-9]|[3-9][0-9])'; then
    info "Instaluję Go"
    GO_VER="$(curl -fsSL 'https://go.dev/VERSION?m=text' | head -1)"
    curl -fsSL "https://go.dev/dl/${GO_VER}.linux-${ARCH}.tar.gz" -o /tmp/go.tgz
    rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tgz && rm -f /tmp/go.tgz
  fi
  export PATH="/usr/local/go/bin:$PATH"
  if [[ ! -f "$SRC_DIR/internal/web/dist/assets" ]] && [[ ! -d "$SRC_DIR/internal/web/dist/assets" ]]; then
    if ! command -v node >/dev/null; then
      info "Instaluję Node.js 22"
      curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
      $APT install nodejs
    fi
    info "Buduję frontend"
    (cd "$SRC_DIR/web" && npm ci && npm run build)
    rm -rf "$SRC_DIR/internal/web/dist" && mkdir -p "$SRC_DIR/internal/web/dist" && cp -r "$SRC_DIR/web/dist/." "$SRC_DIR/internal/web/dist/"
  fi
  info "Buduję olspanel"
  (cd "$SRC_DIR" && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.Version=$(git -C "$SRC_DIR" describe --tags --always 2>/dev/null || echo source)" -o "$PANEL_DIR/bin/olspanel.new" ./cmd/olspanel)
else
  if [[ "$VERSION" == "latest" ]]; then
    VERSION="$(curl -fsSL "https://api.github.com/repos/${OLSPANEL_REPO}/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*//p' | head -1)"; [[ -n "$VERSION" ]] || die "nie można pobrać informacji o wydaniu"
  fi
  BASE="https://github.com/${OLSPANEL_REPO}/releases/download/${VERSION}"
  info "Pobieram olspanel ${VERSION} (${ARCH})"
  curl -fsSL "${BASE}/olspanel-linux-${ARCH}" -o "$PANEL_DIR/bin/olspanel.new"
  curl -fsSL "${BASE}/SHA256SUMS" -o /tmp/olspanel.sums
  (cd "$PANEL_DIR/bin" && grep "olspanel-linux-${ARCH}\$" /tmp/olspanel.sums | sed "s#olspanel-linux-${ARCH}#olspanel.new#" | sha256sum -c -) || die "suma kontrolna binarki nie zgadza się"
fi
chmod 755 "$PANEL_DIR/bin/olspanel.new"
mv -f "$PANEL_DIR/bin/olspanel.new" "$PANEL_DIR/bin/olspanel"
ln -sf "$PANEL_DIR/bin/olspanel" /usr/local/bin/olspanel

# share pages
if [[ -d "$SRC_DIR/configs/share" ]]; then
  cp -r "$SRC_DIR/configs/share/." "$PANEL_DIR/share/"
else
  mkdir -p "$PANEL_DIR/share/default/html" "$PANEL_DIR/share/suspended"
  [[ -f "$PANEL_DIR/share/default/html/index.html" ]] || echo '<!doctype html><title>Serwer działa</title><h1>Serwer działa</h1><p>Ta domena nie jest przypisana do żadnego konta.</p>' > "$PANEL_DIR/share/default/html/index.html"
  [[ -f "$PANEL_DIR/share/suspended/index.html" ]] || echo '<!doctype html><title>Konto zawieszone</title><h1>Konto zawieszone</h1>' > "$PANEL_DIR/share/suspended/index.html"
fi
# OLS refuses docroots owned by uid < 11 (treats it as an error in -t), so hand them to nobody.
chown -R nobody:nogroup "$PANEL_DIR/share"
chmod -R a+rX "$PANEL_DIR/share"

# ---------------------------------------------------------------- phpMyAdmin
if [[ ! -f "$PANEL_DIR/phpmyadmin/index.php" ]]; then
  info "Instaluję phpMyAdmin ${PMA_VERSION}"
  PMA_URL="https://files.phpmyadmin.net/phpMyAdmin/${PMA_VERSION}/phpMyAdmin-${PMA_VERSION}-all-languages.zip"
  curl -fsSL "$PMA_URL" -o /tmp/pma.zip
  curl -fsSL "${PMA_URL}.sha256" -o /tmp/pma.zip.sha256
  (cd /tmp && sed 's#phpMyAdmin-.*-all-languages.zip#pma.zip#' pma.zip.sha256 | sha256sum -c -) || die "suma kontrolna phpMyAdmin nie zgadza się"
  rm -rf /tmp/pma && mkdir -p /tmp/pma && unzip -q /tmp/pma.zip -d /tmp/pma
  rm -rf "$PANEL_DIR/phpmyadmin"
  mv /tmp/pma/phpMyAdmin-*-all-languages "$PANEL_DIR/phpmyadmin"
  rm -rf /tmp/pma /tmp/pma.zip /tmp/pma.zip.sha256
  BLOWFISH="$(openssl rand -base64 32 | tr -d '\n' | cut -c1-32)"
  cat > "$PANEL_DIR/phpmyadmin/config.inc.php" <<EOF
<?php
declare(strict_types=1);
\$cfg['blowfish_secret'] = '${BLOWFISH}';
\$i = 0;
\$i++;
\$cfg['Servers'][\$i]['auth_type'] = 'cookie';
\$cfg['Servers'][\$i]['host'] = 'localhost';
\$cfg['Servers'][\$i]['socket'] = '/run/mysqld/mysqld.sock';
\$cfg['Servers'][\$i]['connect_type'] = 'socket';
\$cfg['Servers'][\$i]['AllowNoPassword'] = false;
\$cfg['Servers'][\$i]['AllowRoot'] = false;
\$cfg['UploadDir'] = '';
\$cfg['SaveDir'] = '';
\$cfg['TempDir'] = '/tmp/olspanel-pma';
\$cfg['PmaAbsoluteUri'] = '/phpmyadmin/';
\$cfg['LoginCookieValidity'] = 3600;
\$cfg['ShowPhpInfo'] = false;
\$cfg['Lang'] = 'pl';
EOF
  mkdir -p /tmp/olspanel-pma && chown nobody:nogroup /tmp/olspanel-pma && chmod 700 /tmp/olspanel-pma
  chmod -R a+rX "$PANEL_DIR/phpmyadmin"
fi

chown -R nobody:nogroup "$PANEL_DIR/phpmyadmin"

# ---------------------------------------------------------------- OLS base config
info "Konfiguruję OpenLiteSpeed"
if ! grep -q 'conf/olspanel/\*.conf' "$LSWS/conf/httpd_config.conf"; then
  [[ -f "$LSWS/conf/httpd_config.conf.pre-olspanel" ]] || cp "$LSWS/conf/httpd_config.conf" "$LSWS/conf/httpd_config.conf.pre-olspanel"
  if [[ -f "$SRC_DIR/configs/ols/httpd_config.base.conf" ]]; then
    cp "$SRC_DIR/configs/ols/httpd_config.base.conf" "$LSWS/conf/httpd_config.conf"
  else
    curl -fsSL "https://raw.githubusercontent.com/${OLSPANEL_REPO}/main/configs/ols/httpd_config.base.conf" -o "$LSWS/conf/httpd_config.conf"
  fi
fi
mkdir -p "$LSWS/conf/olspanel" "$LSWS/conf/vhosts"
chown -R lsadm:lsadm "$LSWS/conf"

# ---------------------------------------------------------------- panel env + init
cat > /etc/olspanel/olspanel.env <<EOF
OLSPANEL_LISTEN=:${PANEL_PORT}
OLSPANEL_HTTP_PORT=${HTTP_PORT}
OLSPANEL_HTTPS_PORT=${HTTPS_PORT}
EOF
HOST_FQDN="${HOSTNAME_OPT:-$(hostname -f 2>/dev/null || hostname)}"
GENERATED_PASS=0
if [[ ! -f "$DATA_DIR/panel.db" && -z "$ADMIN_PASS" ]]; then
  ADMIN_PASS="$(openssl rand -base64 18 | tr -d '/+=' | cut -c1-16)"
  GENERATED_PASS=1
fi
INIT_ARGS=(--hostname "$HOST_FQDN")
[[ -n "$ADMIN_PASS" ]] && INIT_ARGS+=(--admin-password "$ADMIN_PASS")
[[ -n "$ACME_EMAIL" ]] && INIT_ARGS+=(--email "$ACME_EMAIL")
OLSPANEL_DATA_DIR="$DATA_DIR" OLSPANEL_LOG_DIR="$LOG_DIR" OLSPANEL_HTTP_PORT="$HTTP_PORT" OLSPANEL_HTTPS_PORT="$HTTPS_PORT" "$PANEL_DIR/bin/olspanel" init "${INIT_ARGS[@]}"

# pure-ftpd TLS cert = panel cert
if [[ $SKIP_FTP -eq 0 ]]; then
  mkdir -p /etc/ssl/private
  cat "$DATA_DIR/panel-ssl/panel.key" "$DATA_DIR/panel-ssl/panel.crt" > /etc/ssl/private/pure-ftpd.pem
  chmod 600 /etc/ssl/private/pure-ftpd.pem
  systemctl enable --now pure-ftpd
  systemctl restart pure-ftpd
fi

# ---------------------------------------------------------------- systemd unit
if [[ -f "$SRC_DIR/configs/olspanel.service" ]]; then
  cp "$SRC_DIR/configs/olspanel.service" /etc/systemd/system/olspanel.service
else
  curl -fsSL "https://raw.githubusercontent.com/${OLSPANEL_REPO}/main/configs/olspanel.service" -o /etc/systemd/system/olspanel.service
fi
systemctl daemon-reload
systemctl enable --now cron
systemctl enable lshttpd >/dev/null 2>&1 || true
"$LSWS/bin/openlitespeed" -t || die "konfiguracja OpenLiteSpeed jest nieprawidłowa (zobacz $LSWS/logs/error.log)"
systemctl restart lshttpd
systemctl enable olspanel
systemctl restart olspanel

# ---------------------------------------------------------------- firewall
if command -v ufw >/dev/null && { [[ $ENABLE_UFW -eq 1 ]] || ufw status | grep -q "Status: active"; }; then
  info "Otwieram porty w ufw"
  ufw allow 22/tcp >/dev/null
  ufw allow "${HTTP_PORT}"/tcp >/dev/null
  ufw allow "${HTTPS_PORT}"/tcp >/dev/null
  ufw allow ${PANEL_PORT}/tcp >/dev/null
  if [[ $SKIP_FTP -eq 0 ]]; then ufw allow 21/tcp >/dev/null; ufw allow 30000:30100/tcp >/dev/null; fi
  [[ $ENABLE_UFW -eq 1 ]] && ufw --force enable >/dev/null
fi

# ---------------------------------------------------------------- verify
sleep 2
if ! curl -fsk "https://127.0.0.1:${PANEL_PORT}/api/v1/health" >/dev/null; then
  journalctl -u olspanel --no-pager -n 30 || true
  die "panel nie odpowiada na porcie ${PANEL_PORT}"
fi
OLSPANEL_HTTP_PORT="$HTTP_PORT" "$PANEL_DIR/bin/olspanel" doctor || warn "doctor zgłosił braki (patrz wyżej)"

IP="$(curl -fs4 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')"
if [[ $GENERATED_PASS -eq 1 ]]; then
  umask 077
  printf 'login: admin\nhasło: %s\n' "$ADMIN_PASS" > /root/.olspanel_credentials
fi
cat <<EOF

================================================================
  olspanel zainstalowany
  Panel:      https://${IP}:${PANEL_PORT}/   (lub https://${HOST_FQDN}:${PANEL_PORT}/)
  Login:      admin
EOF
if [[ $GENERATED_PASS -eq 1 ]]; then
  echo "  Hasło:      ${ADMIN_PASS}   (zapisane w /root/.olspanel_credentials)"
fi
cat <<EOF
  PHP:        $(ls -d ${LSWS}/lsphp?? | sed 's#.*/lsphp##' | tr '\n' ' ')(domyślnie ${NEWEST_PHP})
  phpMyAdmin: https://${IP}:${PANEL_PORT}/phpmyadmin/
  Log:        ${LOG}
================================================================
EOF
