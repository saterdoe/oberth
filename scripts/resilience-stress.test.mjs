import {test} from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import {runCommand,runStress,stressEnvironment} from './resilience-stress.mjs'

test('stress fails on first flake and preserves evidence, never retries into success', async t => {
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'oberth-stress-contract-'))
  t.after(()=>fs.rmSync(root,{recursive:true,force:true}))
  const seen=[]
  const result=await runStress({count:3,artifactRoot:root,execute:async(iteration,log)=>{
    seen.push(iteration); fs.writeFileSync(log,'fixture-only evidence'); return {passed:iteration!==2}
  }})
  assert.deepEqual(seen,[1,2]); assert.equal(result.passed,false)
  const report=JSON.parse(fs.readFileSync(path.join(result.directory,'report.json'),'utf8'))
  assert.equal(report.first_failed_iteration,2)
  assert.equal(fs.readFileSync(path.join(result.directory,'iteration-2.jsonl'),'utf8'),'fixture-only evidence')
})
test('invalid counts and installed runtime credentials cannot enter a fixture',async()=>{
  await assert.rejects(runStress({count:21}),/iterations/)
  assert.deepEqual(stressEnvironment({PATH:'bin',TEST_DATABASE_URL:'secret',OBERTH_API_TOKEN:'secret',OPENAI_API_KEY:'secret'}),{PATH:'bin',GIT_CONFIG_NOSYSTEM:'1',GIT_CONFIG_GLOBAL:process.platform==='win32'?'NUL':'/dev/null'})
})
test('bounded runner terminates its own hung child and reports timeout',async t=>{
  const root=fs.mkdtempSync(path.join(os.tmpdir(),'oberth-timeout-contract-'))
  t.after(()=>fs.rmSync(root,{recursive:true,force:true}))
  const result=await runCommand(process.execPath,['-e','setInterval(()=>{},1000)'],{directory:root,env:process.env,log:path.join(root,'timeout.log'),timeout:250})
  assert.equal(result.timedOut,true); assert.equal(result.passed,false)
})
