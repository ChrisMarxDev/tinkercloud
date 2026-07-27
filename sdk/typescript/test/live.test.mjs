import assert from "node:assert/strict";
import { LiveChannel, createTiny } from "../dist/index.js";
class FakeSocket { constructor(){this.readyState=0;this.sent=[];this.onopen=this.onclose=this.onerror=this.onmessage=null} send(x){this.sent.push(JSON.parse(x))} close(){this.readyState=3;this.onclose?.({})} open(){this.readyState=1;this.onopen?.({})} message(x){this.onmessage?.({data:x})} }
const sockets=[]; const timers=[];
const options={origin:"https://app.test",webSocket:()=>{const s=new FakeSocket();sockets.push(s);return s},schedule:(fn)=>{timers.push(fn);return fn},cancel:(id)=>{const i=timers.indexOf(id);if(i>=0)timers.splice(i,1)},random:()=>0};
const c=new LiveChannel("updates",options); let resolved=false; const p=c.connect().then(()=>resolved=true); assert.equal(resolved,false); sockets[0].open(); await p; assert.equal(sockets[0].sent[0].type,"subscribe");
let seen=[]; c.onKv("x/",e=>seen.push(e)); sockets[0].message("not json"); sockets[0].message(JSON.stringify({v:1,type:"kv.changed",key:"x/a",version:2,deleted:false})); assert.equal(seen.length,1);
sockets[0].close(); assert.equal(timers.length,1); timers.shift()(); sockets[1].open(); assert.equal(sockets[1].sent[0].type,"subscribe"); c.close(); assert.equal(timers.length,0);

// KV-only subscriptions must not join a reserved custom channel. They need to
// subscribe using the requested prefix and repeat that subscription on reconnect.
const kvSockets=[];
const kvTiny=createTiny({...options,webSocket:()=>{const s=new FakeSocket();kvSockets.push(s);return s}});
const stop=kvTiny.live.onKvChange({prefix:"tasks/"},()=>{});
kvSockets[0].open();
assert.deepEqual(kvSockets[0].sent,[{v:1,type:"subscribe_kv",prefix:"tasks/"}]);
kvSockets[0].close(); timers.shift()(); kvSockets[1].open();
assert.deepEqual(kvSockets[1].sent,[{v:1,type:"subscribe_kv",prefix:"tasks/"}]);
stop();

// Adding a KV listener after the socket opens must subscribe immediately.
const dynamic=new LiveChannel("updates",options); const dynamicConnect=dynamic.connect(); sockets[2].open(); await dynamicConnect;
dynamic.onKv("items/",()=>{});
assert.deepEqual(sockets[2].sent,[
  {v:1,type:"subscribe",channel:"updates"},
  {v:1,type:"subscribe_kv",prefix:"items/"},
]);
dynamic.close();

// Empty lists from an older server may serialize a nil Go slice as null. Apps
// should consistently receive the documented iterable entries array.
const listTiny=createTiny({fetch:async()=>new Response(JSON.stringify({entries:null}),{status:200,headers:{"Content-Type":"application/json"}})});
assert.deepEqual((await listTiny.kv.list({prefix:"tasks/"})).entries,[]);
