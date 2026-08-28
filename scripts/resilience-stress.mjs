import {spawn, spawnSync} from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import {fileURLToPath} from 'node:url'

const root = path.resolve(import.meta.dirname, '..')
export function stressEnvironment(source) {
  const env = {...source}
  for (const key of Object.keys(env)) if (/^(OBERTH_|TEST_DATABASE_URL$|DATABASE_URL$|PGHOST$|PGPASSWORD$|OPENAI_|ANTHROPIC_|OLLAMA_)/i.test(key)) delete env[key]
  return {...env, GIT_CONFIG_NOSYSTEM:'1', GIT_CONFIG_GLOBAL:process.platform === 'win32' ? 'NUL' : '/dev/null'}
}

export function runCommand(command, args, {directory, env, log, timeout = 240000}) {
  return new Promise(resolve => {
    const fd = fs.openSync(log, 'wx')
    const child = spawn(command, args, {cwd:directory, env, windowsHide:true, detached:process.platform !== 'win32', stdio:['ignore',fd,fd]})
    let timedOut = false
    let spawnError = null
    const timer = setTimeout(() => {
      timedOut = true
      // Only the process tree owned by this iteration, never a name-based kill.
      if (child.pid) {
        if (process.platform === 'win32') spawnSync('taskkill', ['/PID',String(child.pid),'/T','/F'], {windowsHide:true, timeout:10000})
        else { try { process.kill(-child.pid, 'SIGKILL') } catch {} }
      }
    }, timeout)
    child.on('error', error => { spawnError = error.message })
    child.on('close', (code, signal) => {
      clearTimeout(timer); fs.closeSync(fd)
      resolve({code, signal, timedOut, spawnError, passed:code === 0 && !timedOut && !spawnError})
    })
  })
}

export async function runStress({count=3, artifactRoot=path.join(root,'artifacts','resilience'), execute}={}) {
  if (!Number.isInteger(count) || count < 1 || count > 20) throw new Error('iterations must be an integer from 1 to 20')
  fs.mkdirSync(artifactRoot,{recursive:true})
  const directory = fs.mkdtempSync(path.join(artifactRoot,'run-'))
  const report = {schema_version:'1',requested_iterations:count,status:'running',iterations:[]}
  const save = () => fs.writeFileSync(path.join(directory,'report.json'), JSON.stringify(report,null,2))
  save()
  for (let iteration=1; iteration<=count; iteration++) {
    console.log(`Resilience iteration ${iteration}/${count}`)
    const log = path.join(directory,`iteration-${iteration}.jsonl`)
    const result = await (execute ? execute(iteration,log) : runCommand('go',['test','-json','-tags','e2e','./internal/api','-run','^Test(DurableRunHTTPHappyPath|ResilienceDatabaseRestart)$','-count=1','-timeout=3m'],{directory:root,env:stressEnvironment(process.env),log}))
    report.iterations.push({iteration,...result})
    if (!result.passed) { report.status='failed'; report.first_failed_iteration=iteration; save(); return {passed:false,directory} }
    save()
  }
  report.status='passed'; save(); return {passed:true,directory}
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const result = await runStress({count:Number(process.argv[2] || 3)})
    console.log(`Stress ${result.passed?'passed':'FAILED'}; evidence: ${result.directory}`)
    if (!result.passed) process.exitCode=1
  } catch (error) { console.error(error.message); process.exitCode=1 }
}
