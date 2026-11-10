const test = require('node:test');
const assert = require('node:assert/strict');
const catalog = require('../Sources/ReflectGo/ui/catalog.js');
const models = [
  {name:'Gemma',developer:'Google',filename:'gemma.gguf',size:17000000000,purpose:'coding-trial',quantization:'Q4_0'},
  {name:'Granite',developer:'IBM',filename:'granite.gguf',size:1000000000,purpose:'chat',quantization:'Q4_K_M'},
];
const options = {query:'',developer:'',purpose:'all',status:'all',maxBytes:Infinity,fitOnly:false,budget:8*2**30,installed:['granite.gguf']};
test('catalog search combines developer, status, purpose, size and hardware filters', () => {
  assert.deepEqual(catalog.filter(models,options).map(m=>m.name),['Granite','Gemma']);
  for (const query of [' IBM ', 'GRANITE', 'q4_k_m']) assert.equal(catalog.filter(models,{...options,query})[0].name,'Granite');
  assert.equal(catalog.filter(models,{...options,developer:'Google',status:'installed'}).length,0);
  assert.equal(catalog.filter(models,{...options,fitOnly:true})[0].name,'Granite');
  assert.equal(catalog.filter(models,{...options,fitOnly:true,budget:0}).length,0);
  assert.equal(catalog.filter(models,{...options,purpose:'coding-trial',maxBytes:2**30}).length,0);
  assert.deepEqual(catalog.filter(models,{...options,status:'download'}).map(m=>m.name),['Gemma']);
});
