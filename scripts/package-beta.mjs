import {readFileSync,writeFileSync,statSync} from "node:fs";
import {join,resolve} from "node:path";
import {createHash} from "node:crypto";
import {spawnSync} from "node:child_process";
const root=resolve(process.argv[3]??".");
const read=p=>JSON.parse(readFileSync(join(root,p),"utf8"));
const pkg=read("package.json");
const expected="0.1.0-beta.2";
if(pkg.name!=="@routilet/accoding-adapter"||pkg.version!==expected)throw Error("package identity mismatch");
const paths={"win32-x64":"bin/win32-x64/accoding-adapter.exe","darwin-arm64":"bin/darwin-arm64/accoding-adapter"};
const hash=p=>createHash("sha256").update(readFileSync(join(root,p))).digest("hex");
const manifest={schemaVersion:1,adapter:"accoding",version:pkg.version,protocol:"routilet-oj-adapter/1",platforms:Object.fromEntries(Object.entries(paths).map(([k,p])=>[k,{path:p,sha256:hash(p)}]))};
if(process.argv[2]==="manifest"){writeFileSync(join(root,"manifest.json"),JSON.stringify(manifest,null,2)+"\n");}
else if(process.argv[2]==="verify"){
 if(JSON.stringify(read("manifest.json"))!==JSON.stringify(manifest))throw Error("manifest/hash mismatch");
 const target=process.platform+"-"+process.arch,rel=paths[target];
 if(!rel)throw Error("unsupported target "+target);
 const executable=join(root,rel);
 if(target==="darwin-arm64"){
  if(!(statSync(executable).mode&0o111))throw Error("not executable");
  const signed=spawnSync("codesign",["--verify","--strict",executable],{timeout:10000});if(signed.status!==0)throw Error("signature invalid");
 }
 const requests=[{id:1,method:"capabilities",params:{protocol:manifest.protocol}},{id:2,method:"account",params:{refresh:false}}];
 const run=spawnSync(executable,["--stdio"],{input:requests.map(x=>JSON.stringify(x)).join("\n")+"\n",encoding:"utf8",timeout:10000,windowsHide:true,maxBuffer:65536});
 if(run.error||run.status!==0||run.stderr)throw Error("offline smoke failed");
 const replies=run.stdout.trimEnd().split("\n").map(JSON.parse);
 const methods=["capabilities","account","login","logout","resolve","import","prepare","submit","reconcile","result","status","import_document","contests","status_detail","result_detail","contest_status"];
 if(replies.length!==2||replies[0].id!==1||replies[0].error||replies[0].result.version!==expected||replies[0].result.adapter!=="accoding"||replies[0].result.protocol!==manifest.protocol||JSON.stringify(replies[0].result.methods)!==JSON.stringify(methods)||replies[1].id!==2||replies[1].error||replies[1].result.state!=="signed_out")throw Error("offline handshake failed");
 console.log("Verified "+pkg.name+"@"+pkg.version+" "+target);
}else throw Error("expected manifest or verify");
