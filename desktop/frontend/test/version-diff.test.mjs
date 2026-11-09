import test from "node:test";
import assert from "node:assert/strict";
import {diffLines} from "../src/versionDiff.ts";
test("version differences preserve both original texts",()=>{
 for(const [before,after] of [["a\nb\nc","a\nnew\nc"],["","new"],["gone",""],["a\nb\na","a\na"],["same","same"],["x\n".repeat(600),"y\n".repeat(600)]]){
 const lines=diffLines(before,after);assert.equal(lines.filter(l=>l.kind!=="added").map(l=>l.text).join("\n"),before);assert.equal(lines.filter(l=>l.kind!=="removed").map(l=>l.text).join("\n"),after);
 }
 assert.deepEqual(diffLines("a\nb\nc","a\nnew\nc").map(l=>l.kind),["same","removed","added","same"]);
});
