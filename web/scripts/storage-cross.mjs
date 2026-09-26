import assert from 'node:assert/strict';import { execFileSync } from 'node:child_process';import { readFile } from 'node:fs/promises';import { fileURLToPath } from 'node:url';
import { canonicalJSON,digest,parseManifest } from '../src/lib/packmanifest.ts';
const fixture=fileURLToPath(new URL('../../protocol/packmanifest/vectors.json',import.meta.url));const f=JSON.parse(await readFile(fixture,'utf8'));
const go=JSON.parse(execFileSync('go',['run','./internal/packmanifest/cmd/vectors',fixture],{cwd:fileURLToPath(new URL('../../cli/',import.meta.url)),encoding:'utf8',timeout:120000}));
for(const [i,v] of f.vectors.entries()){
 const b=canonicalJSON(new TextEncoder().encode(v.input));assert.deepEqual(go[i],{name:v.name,canonical:new TextDecoder().decode(b),sha256:await digest(b)});
 if(v.kind==='manifest')await parseManifest(new TextEncoder().encode(go[i].canonical),v.context,{...v.commitment,sha256:go[i].sha256});
}console.log('PASS: Go -> TS and TS -> Go canonical bytes/digests/context:',go.length,'vectors');
