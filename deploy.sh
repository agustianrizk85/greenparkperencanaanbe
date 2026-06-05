#!/usr/bin/env bash
# Deploy backend Perencanaan: tarik kode terbaru, build, jalankan/-ulang via PM2.
# Jalankan di server dari dalam folder repo: ./deploy.sh
set -euo pipefail
cd "$(dirname "$0")"

echo "==> git pull"
git pull --ff-only

echo "==> go build"
export PATH="$PATH:/usr/local/go/bin"
CGO_ENABLED=0 go build -trimpath -o perencanaan-server ./cmd/server

# Muat env (port) dari file di luar git bila ada.
set -a; [ -f /opt/apps/perencanaan.env ] && . /opt/apps/perencanaan.env; set +a

echo "==> (re)start PM2: perencanaan-be"
pm2 restart perencanaan-be --update-env 2>/dev/null || pm2 start ./perencanaan-server --name perencanaan-be --update-env
pm2 save
echo "==> selesai. status:"
pm2 status perencanaan-be
