import test from 'node:test';import assert from 'node:assert/strict';import { readFile } from 'node:fs/promises';
import { canonicalJSON, strictJSON, parseManifest, encodeManifest, digest } from '../src/lib/packmanifest.ts';
const f=JSON.parse(await readFile(new URL('../../protocol/packmanifest/vectors.json',import.meta.url),'utf8'));const enc=new TextEncoder(),dec=new TextDecoder();
for(const v of f.vectors)test('manifest fixture '+v.name,async()=>{
 const b=canonicalJSON(enc.encode(v.input));assert.equal(dec.decode(b),v.canonical);assert.equal(await digest(b),v.sha256);
 if(v.kind==='manifest'){
  const m=await parseManifest(b,v.context,v.commitment);assert.deepEqual(encodeManifest(m),b);
  await assert.rejects(parseManifest(b,{...v.context,repoId:'0x'+'f'.repeat(64)},v.commitment));
  await assert.rejects(parseManifest(b,v.context,{...v.commitment,sha256:'0'.repeat(64)}));
  await assert.rejects(parseManifest(b,v.context,{...v.commitment,size:'1'}));
  const noncanonical=enc.encode(JSON.stringify(m,null,2));await assert.rejects(parseManifest(noncanonical,v.context,{...v.commitment,sha256:await digest(noncanonical),size:String(noncanonical.length)}));
 }
});
for(const v of f.invalid)test('reject fixture '+v.name,async()=>{
 if(v.kind==='jcs'){assert.throws(()=>strictJSON(enc.encode(v.input)));return;}
 const b=canonicalJSON(enc.encode(v.input));await assert.rejects(parseManifest(b,f.vectors[3].context,{sha256:await digest(b),size:String(b.length),bootstrapLocator:'https://storage.example.com/manifest'}));
});
test('invalid UTF8 and limits',()=>{assert.throws(()=>strictJSON(Uint8Array.of(255)));assert.throws(()=>strictJSON(enc.encode(' '.repeat(65536)+'{}')));});
