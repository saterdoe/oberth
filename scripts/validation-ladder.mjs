import {spawnSync} from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import {fileURLToPath} from 'node:url'

const root=path.resolve(import.meta.dirname,'..')
export const gates=[
  {id:'contracts',invariant:'Versioned actions and evidence reject malformed or unsupported claims',dependsOn:[],args:['test','-json','./internal/agentruntime','./internal/reasoning','./internal/structuredoutput']},
  {id:'components',invariant:'Workspace boundaries, providers and recovery contracts hold independently',dependsOn:['contracts'],args:['test','-json','./internal/workspace','./internal/toolrunner','./internal/gateway','./internal/recovery']},
  {id:'durable-flow',invariant:'Plan, isolated edits, evidence, decisions, promotion and recovery use the real HTTP runtime',dependsOn:['components'],args:['test','-json','-tags','e2e','./internal/api','-run','^TestDurableRunHTTPHappyPath$','-count=1','-timeout=5m']},
]

export function offlineEnvironment(env,distribution,artifacts){
  const safe={...env}
  for(const key of Object.keys(safe))if(/^(OBERTH_|TEST_DATABASE_URL$|DATABASE_URL$|PGHOST$|PGPASSWORD$|OPENAI_|ANTHROPIC_|GOOGLE_API_KEY$|OLLAMA_)/i.test(key))delete safe[key]
  return {...safe,GOPROXY:'off',GOSUMDB:'off',GOTOOLCHAIN:'local',GIT_CONFIG_NOSYSTEM:'1',GIT_CONFIG_GLOBAL:process.platform==='win32'?'NUL':'/dev/null',GIT_AUTHOR_DATE:'2026-01-01T00:00:00Z',GIT_COMMITTER_DATE:'2026-01-01T00:00:00Z',OBERTH_E2E_POSTGRES_BIN:distribution,OBERTH_E2E_ARTIFACT_DIR:artifacts}
}

export function runLadder({prepare=false}={}){
  const cache=path.join(root,'artifacts','hermetic-postgres')
  const distribution=path.join(cache,'v16','binaries')
  if(prepare){
    for(const args of [['mod','download'],['test','-tags','e2e','./internal/api','-run','^TestPrepareHermeticDatabase$','-count=1']]){
      const result=spawnSync('go',args,{cwd:root,env:{...process.env,OBERTH_E2E_PREPARE_DIR:cache},stdio:'inherit'})
      if(result.status!==0)throw new Error('Dependency preparation failed')
    }
    console.log(`Prepared offline distribution: ${distribution}`)
    return
  }
  const artifactRoot=path.join(root,'artifacts','validation')
  fs.mkdirSync(artifactRoot,{recursive:true})
  const artifacts=fs.mkdtempSync(path.join(artifactRoot,'run-'))
  const report={schema_version:'1',status:'running',first_broken_gate:null,gates:[],artifacts}
  const save=()=>fs.writeFileSync(path.join(artifacts,'report.json'),JSON.stringify(report,null,2))
  if(!fs.existsSync(path.join(distribution,'bin','pg_ctl'))){report.status='failed';report.first_broken_gate='prerequisites';save();throw new Error(`Prepare dependencies first: node scripts/validation-ladder.mjs --prepare. Report: ${artifacts}`)}
  const env=offlineEnvironment(process.env,distribution,artifacts)
  for(const gate of gates){
    console.log(`[${gate.id}] ${gate.invariant}`)
    const result=spawnSync('go',gate.args,{cwd:root,env,encoding:'utf8',timeout:360000,maxBuffer:32*1024*1024})
    fs.writeFileSync(path.join(artifacts,`${gate.id}.jsonl`),result.stdout||'')
    fs.writeFileSync(path.join(artifacts,`${gate.id}.stderr.log`),result.stderr||result.error?.message||'')
    const markers=(result.stdout||'').split('\n').flatMap(line=>{try{const event=JSON.parse(line);return event.Output?.includes('GATE ')?[event.Output.trim()]:[]}catch{return []}})
    report.gates.push({...gate,status:result.status===0?'passed':'failed',last_invariant:markers.at(-1)||gate.invariant})
    if(result.status!==0){report.status='failed';report.first_broken_gate=gate.id;save();throw new Error(`First broken gate: ${gate.id}; ${markers.at(-1)||gate.invariant}. Artifacts: ${artifacts}`)}
    save()
  }
  report.status='passed';save();console.log(`Validation ladder passed. Report: ${artifacts}`)
}

if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url)){
  try{runLadder({prepare:process.argv.includes('--prepare')})}catch(error){console.error(error.message);process.exitCode=1}
}
