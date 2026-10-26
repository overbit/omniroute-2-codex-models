'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const http = require('node:http');
const { convertCatalog, handleRequest } = require('./proxy');
test('translates and deduplicates', () => {
 const models = convertCatalog({data:[{id:'alpha',context_window:12000},{id:'alpha'},{id:'beta'}]}).models;
 assert.deepEqual(models.map(m=>m.slug), ['alpha','beta']);
 assert.equal(models[0].context_window,12000);
 assert.ok(models[0].model_messages.instructions_template);
});
test('rejects empty catalog', () => assert.throws(()=>convertCatalog({data:[]}), /no usable models/));
test('forwards bearer token', async () => {
 const upstream=http.createServer((req,res)=>{assert.equal(req.headers.authorization,'Bearer test');res.end(JSON.stringify({data:[{id:'alpha'}]}));});
 await new Promise(r=>upstream.listen(0,'127.0.0.1',r));
 const adapter=http.createServer(handleRequest);
 await new Promise(r=>adapter.listen(0,'127.0.0.1',r));
 const prev=process.env.OMNIROUTE_MODELS_URL;
 process.env.OMNIROUTE_MODELS_URL='http://127.0.0.1:'+upstream.address().port+'/v1/models';
 try {
   const response=await fetch('http://127.0.0.1:'+adapter.address().port+'/models',{headers:{Authorization:'Bearer test'}});
   assert.equal(response.status,200);
   assert.equal((await response.json()).models[0].slug,'alpha');
 } finally {
   if(prev===undefined) delete process.env.OMNIROUTE_MODELS_URL; else process.env.OMNIROUTE_MODELS_URL=prev;
   await Promise.all([new Promise(r=>adapter.close(r)),new Promise(r=>upstream.close(r))]);
 }
});
