import assert from "node:assert/strict";
import { createTinker, TinkerInvalidRequestError, TinkerQuotaExhaustedError, TinkerTemporarilyUnavailableError } from "../dist/index.js";

let captured;
const tinker = createTinker({ fetch: async (path, init) => { captured={path,init}; return new Response(JSON.stringify({message:{role:"assistant",content:"A concise answer."},usage:{input_tokens:12,output_tokens:4},finish_reason:"stop",request_id:"req_123"}),{status:200,headers:{"Content-Type":"application/json"}}); } });
const answer=await tinker.llm.chat.complete({messages:[{role:"user",content:"Summarize this."}],maxOutputTokens:20});
assert.equal(captured.path,"/_tinker/api/v1/llm/chat");assert.equal(captured.init.method,"POST");assert.deepEqual(JSON.parse(captured.init.body),{messages:[{role:"user",content:"Summarize this."}],max_output_tokens:20});assert.deepEqual(answer,{message:{role:"assistant",content:"A concise answer."},usage:{inputTokens:12,outputTokens:4},finishReason:"stop",requestId:"req_123"});
await tinker.llm.chat.complete({messages:[{role:"user",content:"Use the profile default."}]});
assert.deepEqual(JSON.parse(captured.init.body),{messages:[{role:"user",content:"Use the profile default."}]},"undefined public fields are omitted from the wire request");
const errorTinker=createTinker({fetch:async()=>new Response(JSON.stringify({error:{code:"invalid_request",message:"safe"}}),{status:400,headers:{"Content-Type":"application/json"}})});await assert.rejects(()=>errorTinker.llm.chat.complete({messages:[]}),TinkerInvalidRequestError);
const quotaTinker=createTinker({fetch:async()=>new Response(JSON.stringify({error:{code:"quota_exhausted",message:"safe"}}),{status:429,headers:{"Content-Type":"application/json"}})});await assert.rejects(()=>quotaTinker.llm.chat.complete({messages:[]}),TinkerQuotaExhaustedError);
const malformed=createTinker({fetch:async()=>new Response(JSON.stringify({message:{role:"user",content:"no"},usage:{input_tokens:-1,output_tokens:2},finish_reason:"stop",request_id:"x"}),{status:200,headers:{"Content-Type":"application/json"}})});await assert.rejects(()=>malformed.llm.chat.complete({messages:[]}),TinkerTemporarilyUnavailableError);
