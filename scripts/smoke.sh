#!/usr/bin/env bash
#
# Smoke test against a freshly installed olspanel on this host (run as root).
# Creates a package, a user, a domain, a database, an FTP account and a cron
# job through the REST API, then verifies each one at the OS level and
# removes the user again.
#
set -euo pipefail

BASE="${BASE:-https://127.0.0.1:2222}"
command -v python3 >/dev/null || apt-get install -y -q python3 >/dev/null
ADMIN_PASS="${ADMIN_PASS:-Test1234!}"
JAR="$(mktemp)"
trap 'rm -f "$JAR"' EXIT
CSRF=""

api() { # method path [json]
  local m="$1" p="$2" d="${3:-}"
  if [[ -n "$d" ]]; then
    curl -fsk -b "$JAR" -c "$JAR" -X "$m" -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" "$BASE/api/v1$p" -d "$d"
  else
    curl -fsk -b "$JAR" -c "$JAR" -X "$m" -H "X-CSRF-Token: $CSRF" "$BASE/api/v1$p"
  fi
}
json() { python3 -c "import sys,json; d=json.load(sys.stdin); print(eval('d$1'))"; }
step() { echo "--> $*"; }

step "health"
api GET /health | grep -q '"ok":true'

step "login as admin"
CSRF="$(api POST /auth/login "{\"username\":\"admin\",\"password\":\"$ADMIN_PASS\"}" | json "['csrf']")"
[[ -n "$CSRF" ]]

step "create package"
PKG="$(api POST /admin/packages '{"name":"Smoke","disk_mb":1024,"max_domains":2,"max_subdomains":2,"max_databases":2,"max_ftp":2,"max_cron":2,"php_versions":["'"$(ls -d /usr/local/lsws/lsphp?? | sort | tail -1 | sed 's#.*/lsphp##')"'"]}' | json "['id']")"

step "create user smoke1"
UID_="$(api POST /admin/users "{\"username\":\"smoke1\",\"password\":\"Smoke12345!\",\"email\":\"s@example.com\",\"role\":\"user\",\"package_id\":$PKG}" | json "['id']")"
id smoke1 >/dev/null
[[ -d /home/smoke1/domains ]]

step "impersonate user"
CSRF="$(api POST "/admin/users/$UID_/impersonate" | json "['csrf']")"

step "create domain"
DOM="$(api POST /domains '{"name":"smoke.test","type":"domain"}' | json "['id']")"
[[ -f /usr/local/lsws/conf/vhosts/smoke.test/vhconf.conf ]]
grep -q "map                     smoke.test smoke.test, www.smoke.test" /usr/local/lsws/conf/olspanel/20-listeners.conf
/usr/local/lsws/bin/openlitespeed -t

step "PHP executes as the account user"
echo '<?php echo "user=" . get_current_user() . " php=" . PHP_VERSION;' > /home/smoke1/domains/smoke.test/public_html/t.php
chown smoke1:smoke1 /home/smoke1/domains/smoke.test/public_html/t.php
sleep 1
OUT="$(curl -fs -H 'Host: smoke.test' http://127.0.0.1/t.php)"
echo "    $OUT"
echo "$OUT" | grep -q 'user=smoke1'

step "static file served"
curl -fs -H 'Host: smoke.test' http://127.0.0.1/ | grep -qi 'smoke.test'

step "create database + user"
DBID="$(api POST /databases '{"suffix":"app"}' | json "['id']")"
api POST "/databases/$DBID/users" '{"suffix":"app","password":"DbPass12345!"}' >/dev/null
mysql -u smoke1_app -pDbPass12345! -e 'SELECT 1' smoke1_app >/dev/null

step "create ftp account"
api POST /ftp '{"suffix":"","password":"FtpPass12345!","home_subdir":"domains/smoke.test/public_html"}' >/dev/null
pure-pw show smoke1 -f /etc/pure-ftpd/pureftpd.passwd >/dev/null
curl -fs --ssl-reqd -k --list-only -u 'smoke1:FtpPass12345!' ftp://127.0.0.1/ | grep -q 't.php' || curl -fs --list-only -u 'smoke1:FtpPass12345!' ftp://127.0.0.1/ | grep -q 't.php'

step "create cron job"
api POST /cron '{"schedule":"*/5 * * * *","command":"echo smoke","enabled":true}' >/dev/null
crontab -u smoke1 -l | grep -q 'echo smoke'

step "file manager"
api GET '/files?path=domains/smoke.test/public_html' | grep -q 't.php'
api POST /files/mkdir '{"path":"domains/smoke.test/public_html/dir1"}' >/dev/null
[[ -d /home/smoke1/domains/smoke.test/public_html/dir1 ]]
[[ "$(stat -c %U /home/smoke1/domains/smoke.test/public_html/dir1)" == "smoke1" ]]
! api GET '/files?path=../../etc' >/dev/null 2>&1

step "stop impersonation, delete user"
CSRF="$(api POST /auth/stop-impersonation | json "['csrf']")"
api DELETE "/admin/users/$UID_" >/dev/null
! id smoke1 >/dev/null 2>&1
[[ ! -f /usr/local/lsws/conf/vhosts/smoke.test/vhconf.conf ]]
! mysql -e 'USE smoke1_app' >/dev/null 2>&1
api DELETE "/admin/packages/$PKG" >/dev/null
/usr/local/lsws/bin/openlitespeed -t

echo "SMOKE OK"
