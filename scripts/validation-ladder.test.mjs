import {test} from 'node:test'
import assert from 'node:assert/strict'
import {gates,offlineEnvironment} from './validation-ladder.mjs'

test('every gate names its invariant and only depends on earlier gates',()=>{
  const seen=new Set()
  for(const gate of gates){assert.ok(gate.invariant.length>20);for(const id of gate.dependsOn)assert.ok(seen.has(id));assert.ok(!seen.has(gate.id));seen.add(gate.id)}
})
test('offline environment cannot inherit personal database or provider credentials',()=>{
  const env=offlineEnvironment({PATH:'tools',TEST_DATABASE_URL:'personal',DATABASE_URL:'personal',OBERTH_AUTH_TOKEN:'secret',OPENAI_API_KEY:'secret',OLLAMA_HOST:'remote'},'distribution','artifacts')
  assert.equal(env.PATH,'tools');assert.equal(env.GOPROXY,'off');assert.equal(env.GOTOOLCHAIN,'local')
  for(const name of ['TEST_DATABASE_URL','DATABASE_URL','OBERTH_AUTH_TOKEN','OPENAI_API_KEY','OLLAMA_HOST'])assert.equal(env[name],undefined)
})
