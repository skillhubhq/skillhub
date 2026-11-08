import test from "node:test";
import assert from "node:assert/strict";
import { safeProvenanceURL, validProvenance, provenanceInfo, provenanceWire } from "../src/provenance.ts";
const p={author_name:"Example Authors",author_url:"https://example.org/authors",source_url:"https://example.org/source/"+"a".repeat(40),revision:"a".repeat(40),license:"MIT",license_file:"LICENSE.txt",modified:false,changes:""};
test("Web and native metadata have the same shape",()=>{assert.deepEqual(provenanceInfo(provenanceWire(p)),p);assert.equal(provenanceInfo(undefined),undefined);assert.equal(validProvenance(p,["LICENSE.txt"]),true);assert.equal(validProvenance(p,[]),false);assert.equal(validProvenance({...p,modified:true},["LICENSE.txt"]),false);});
test("untrusted Hub links cannot open scripts or forward embedded credentials",()=>{for(const s of ["javascript:alert(1)","file:///tmp/private","https://user:pass@example.org","https://example.org?token=secret","https://example.org\n/path"])assert.equal(safeProvenanceURL(s),"");assert.equal(safeProvenanceURL(p.source_url),p.source_url);});
