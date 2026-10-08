#!/usr/bin/env bash
#
# End-to-end test of install.sh inside a fresh, disposable WSL2 distribution.
# Run from Git Bash / PowerShell on Windows:
#
#   bash scripts/test-install-wsl.sh            # Ubuntu 24.04 (default)
#   UBUNTU=26.04 bash scripts/test-install-wsl.sh
#   KEEP=1 bash scripts/test-install-wsl.sh      # keep the distro afterwards
#
# Requirements: WSL2, internet access. The distro is named olspanel-test-<ver>.
#
set -euo pipefail

UBUNTU="${UBUNTU:-24.04}"
DISTRO="olspanel-test-${UBUNTU//./}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WSL_BASE="${WSL_BASE:-$LOCALAPPDATA/olspanel-wsl}"
mkdir -p "$WSL_BASE"

case "$UBUNTU" in
  24.04) ROOTFS_URL="https://cloud-images.ubuntu.com/wsl/releases/24.04/current/ubuntu-24.04-wsl-amd64.wsl" ;;
  26.04) ROOTFS_URL="https://cloud-images.ubuntu.com/wsl/releases/26.04/current/ubuntu-26.04-wsl-amd64.wsl" ;;
  *) echo "unsupported UBUNTU=$UBUNTU"; exit 1 ;;
esac
ROOTFS="$WSL_BASE/ubuntu-${UBUNTU}.wsl"

if ! wsl.exe -l -q | tr -d '\r\0' | grep -qx "$DISTRO"; then
  if [[ ! -f "$ROOTFS" ]]; then
    echo "==> downloading $ROOTFS_URL"
    curl -fL --progress-bar "$ROOTFS_URL" -o "$ROOTFS"
  fi
  echo "==> importing $DISTRO"
  wsl.exe --import "$DISTRO" "$(cygpath -w "$WSL_BASE/$DISTRO")" "$(cygpath -w "$ROOTFS")" --version 2
  wsl.exe -d "$DISTRO" -u root -- bash -c 'printf "[boot]\nsystemd=true\n[user]\ndefault=root\n" > /etc/wsl.conf'
  wsl.exe --terminate "$DISTRO"
fi

WIN_ROOT="$(cygpath -w "$ROOT_DIR")"
echo "==> copying repository into the distro"
wsl.exe -d "$DISTRO" -u root -- bash -c "rm -rf /root/olspanel && mkdir -p /root/olspanel && cp -r \"\$(wslpath '$WIN_ROOT')\"/. /root/olspanel/ && rm -rf /root/olspanel/web/node_modules /root/olspanel/bin"

echo "==> running install.sh --from-source"
wsl.exe -d "$DISTRO" -u root -- bash -c 'cd /root/olspanel && bash install.sh --from-source --admin-password "Test1234!" --hostname panel.test --email test@example.com'

echo "==> smoke test"
wsl.exe -d "$DISTRO" -u root -- bash -c 'cd /root/olspanel && bash scripts/smoke.sh'

if [[ "${KEEP:-0}" != "1" ]]; then
  echo "==> removing $DISTRO (set KEEP=1 to keep it)"
  wsl.exe --unregister "$DISTRO" >/dev/null
fi
echo "==> OK"
