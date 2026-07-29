#!/usr/bin/env bash
set -uo pipefail
cd /mnt/d/next-injective-git/web/node_modules/@injectivelabs
echo "== EvmChainId location + members =="
grep -rlE "EvmChainId" ts-types/dist/esm 2>/dev/null | head -3
grep -rhoE "EvmChainId[^;]*Testnet[^;]*|Testnet = [0-9]+|Injective = [0-9]+" ts-types/dist/esm 2>/dev/null | head
echo
echo "== getEip712TypedData signature =="
grep -rhA3 "getEip712TypedData" sdk-ts/dist/esm/**/eip712*.d.ts 2>/dev/null | head -20
echo
echo "== recoverTypedSignaturePubKey signature =="
grep -rhA2 "recoverTypedSignaturePubKey" sdk-ts/dist/esm/**/*.d.ts 2>/dev/null | head -8
echo
echo "== createWeb3Extension signature =="
grep -rhA4 "createWeb3Extension" sdk-ts/dist/esm/**/*.d.ts 2>/dev/null | head -12
echo
echo "== MsgExecuteContract.fromJSON param shape =="
find sdk-ts/dist/esm -name "MsgExecuteContract.d.ts" -exec grep -hA30 "namespace MsgExecuteContract\|interface Params" {} \; 2>/dev/null | head -40
