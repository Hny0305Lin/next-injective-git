#!/usr/bin/env bash
cd /mnt/d/next-injective-git/web/node_modules
for p in \
  cosmjs-types/cosmwasm/wasm/v1/tx.js \
  cosmjs-types/cosmos/tx/v1beta1/tx.js \
  cosmjs-types/cosmos/tx/signing/v1beta1/signing.js \
  cosmjs-types/cosmos/crypto/secp256k1/keys.js \
  cosmjs-types/google/protobuf/any.js \
  @cosmjs/encoding/build/index.js; do
  if [ -f "$p" ]; then echo "OK   $p"; else echo "MISS $p"; fi
done
