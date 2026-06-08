#!/usr/bin/env bash
set -euo pipefail

SRC_URL="${MIDTRANS_POSTMAN_COLLECTION_URL:-https://raw.githubusercontent.com/midtrans/Midtrans-Payment-API-Postman-Collections/master/Midtrans%20Payment%20API.postman_collection.json}"
OUT_FILE="${1:-docs/apis/midtrans-openapi-from-postman.json}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ "$OUT_FILE" = /* ]]; then
  OUT_PATH="$OUT_FILE"
else
  OUT_PATH="$ROOT_DIR/$OUT_FILE"
fi
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

mkdir -p "$(dirname "$OUT_PATH")"
COLLECTION_PATH="$TMP_DIR/postman.json"
NODE_SCRIPT="$TMP_DIR/convert.js"

cat > "$NODE_SCRIPT" <<'NODE'
const fs = require('fs');

const srcPath = process.argv[2];
if (!srcPath) {
  throw new Error('missing source collection path');
}

const rawData = fs.readFileSync(srcPath, 'utf8');
const collection = JSON.parse(rawData);

function toPath(rawUrl) {
  if (typeof rawUrl !== 'string' || rawUrl.trim() === '') {
    return '';
  }

  let path = rawUrl.replace(/^https?:\/\/[^/]+/i, '');
  path = path.split('?')[0];

  if (path === '') {
    path = '/';
  }
  if (!path.startsWith('/')) {
    path = `/${path}`;
  }

  return path
    .replace(/\[INSERT-ORDER-ID\]/g, '{order_id}')
    .replace(/\[ORDER-ID\]/g, '{order_id}')
    .replace(/\[[^\]]+\]/g, '{param}');
}

function summarizeName(name) {
  return (name || 'api_operation')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '');
}

function collectItems(root) {
  const output = [];
  const walk = (nodes) => {
    if (!Array.isArray(nodes)) {
      return;
    }
    for (const node of nodes) {
      if (!node || typeof node !== 'object') {
        continue;
      }
      if (Array.isArray(node.item)) {
        walk(node.item);
        continue;
      }
      const request = node.request;
      if (!request || !request.url) {
        continue;
      }
      const method = (request.method || 'GET').toUpperCase();
      const rawUrl = typeof request.url === 'string' ? request.url : (request.url.raw || '');
      const path = toPath(rawUrl);
      if (!path) {
        continue;
      }
      output.push({
        method,
        path,
        name: node.name || `${method} ${path}`,
        key: `${method.toLowerCase()} ${path}`,
      });
    }
  };

  walk(root.item);
  return output;
}

const items = collectItems(collection);
items.sort((a, b) => {
  const pathCmp = a.path.localeCompare(b.path);
  if (pathCmp !== 0) return pathCmp;
  return a.method.localeCompare(b.method);
});

const unique = new Map();
for (const item of items) {
  if (!unique.has(item.key)) {
    unique.set(item.key, item);
  }
}

const paths = {};
for (const item of unique.values()) {
  if (!paths[item.path]) {
    paths[item.path] = {};
  }
  const pathTag = item.path.replace(/^\//, '').replace(/\//g, '_') || 'root';
  const operationId = `${item.method.toLowerCase()}_${pathTag}_${summarizeName(item.name)}`.slice(0, 96);

  paths[item.path][item.method.toLowerCase()] = {
    operationId,
    summary: item.name,
    responses: {
      '200': { description: 'Success' },
      default: { description: 'Error' },
    },
  };
}

const spec = {
  openapi: '3.0.3',
  info: {
    title: 'Midtrans API (from Postman collection)',
    version: '2019-01',
    description: 'Converted from Midtrans Payment API Postman collection. Validate and enrich before production use.',
  },
  servers: [
    { url: 'https://api.sandbox.midtrans.com', description: 'Midtrans Sandbox' },
    { url: 'https://api.midtrans.com', description: 'Midtrans Production' },
  ],
  paths,
};

process.stdout.write(JSON.stringify(spec, null, 2));
NODE

curl -fsSL "$SRC_URL" -o "$COLLECTION_PATH"
node - "$COLLECTION_PATH" < "$NODE_SCRIPT" > "$OUT_PATH"
"$ROOT_DIR/scripts/generate-midtrans-openapi-aliases.sh" "$OUT_PATH" "$ROOT_DIR/internal/cli/midtrans_openapi_aliases_generated.go"
printf 'wrote %s\n' "$OUT_PATH"
