import { useEffect, useMemo, useState, useRef } from 'react'
import {
  BookOpen, Check, Coins, FileText, Folder, LayoutDashboard, LoaderCircle,
  Route, Search, Settings, SquareTerminal, X,
} from 'lucide-react'
import ProductTour,{TourTarget} from './ProductTour'
import Modal from './Modal'
import { useWebSocket } from './useWebSocket'
import { applyTaskStreamEvent } from './taskStream'
import { apiBase, apiToken, isDesktop, pickNativeDirectory, pickNativeRepository } from './runtimeConfig'
import {MessageKey,useI18n} from './i18n'

type Status = {
  status?: string
  server?: { state?: string; uptime?: string; version?: string }
  vault?: { state?: string; note_count?: number; last_indexed?: string | null }
  vector_store?: { state?: string; engine?: string; embedder?:string; model?:string; dimensions?:number }
  database?: { state?: string }
}
type Provider = { id:string; name:string; provider_type:string; is_active:boolean; base_url?:string; default_model?:string; models?:string|string[]; capabilities?:{typed_actions_certified_models?:string[]} }
type LocalProviderCandidate = { id:string;name:string;kind?:'inference-provider'|'agent-harness';provider_type?:string;base_url?:string;installed:boolean;running:boolean;usable:boolean;models:string[];message:string;version?:string;auth?:string;evidence?:string[] }
type Session = { id:string; task_id?:string; repo_path?:string; task_type:string; task_description?:string; status:string; model?:string; tokens_input:number; tokens_output:number; cost:number; started_at:string; plan?:unknown }
type WorkflowStage = { id:string; role:'analysis'|'documentation'|'development'|'qa'|'review'; provider_id:string; model:string }
type Task = { id:string; repository_id?:string; title:string; description:string; task_type:string; constraints?:unknown; status:string; created_at:string; updated_at:string }
type Run = { id:string; task_id:string; session_id:string; state:string; schema_version:string; started_at?:string; finished_at?:string; base_repository?:string; base_commit?:string; worktree_path?:string; branch?:string; result_bundle?:Record<string,unknown> }
type RunEvent = { sequence:number; type:string; payload:Record<string,unknown>; time:string; schema_version:string }
type CostSummary = { total_cost?:number; total_tokens?:number; total_calls?:number; by_provider?:Record<string,number> }
type VaultNote = { path?:string; name?:string; content?:string; metadata?:Record<string,unknown>; relevance?:number;reason?:string }
type VaultSearchResponse = {results:Array<{note:VaultNote;score:number;reason?:string}>;metrics?:{mode?:string;semantic_used?:boolean;keyword_fallback?:boolean;fused_candidates?:number}}
type SemanticSettings = {enabled:boolean;engine:string;embedder:string;model:string;dimensions:number;migration:string;qdrant_url?:string;collection?:string}
type MemoryCandidate = { id:string;kind:string;claim_id?:string;content:string;source_commit:string;created_by:string;confidence:number;scope:string;status:string;created_at:string;evidence_ids?:string[];validity_status?:'current'|'needs_revalidation'|'contradicted'|'superseded';supersedes?:string;contradicts?:string }
type RouteRule = { id:string; priority?:number; name?:string; match_repo_pattern?:string; match_task_type?:string; provider_id?:string; model?:string; is_active?:boolean }
type Project = { id:string; name:string; path:string }
type PickedRepository = { canceled:boolean; name?:string; path?:string }
type DiffFile = { path:string; status:string; content:string }
type PromotionReadiness = { ready:boolean; reason?:string }
type Launcher = { id:string;name:string;available:boolean;message:string }
export type ReasoningRecord = {id:string;kind:'fact'|'hypothesis'|'assumption'|'unknown'|'property'|'decision';statement:string;status:string;confidence?:number;evidence_ids?:string[];falsifier?:string;scope?:string;required?:boolean;next_action?:string}
export type ReasoningEvidence = {id:string;source:string;hash?:string;subject?:string;subject_hash?:string;detail?:string;stale?:boolean}
export type ReasoningExperiment = {id:string;question:string;preconditions?:string[];environment:string;command:string;expectation:string;observation:string;status:'passed'|'failed'|'unknown';duration_ms?:number;cost?:number;evidence_ids:string[];claim_ids?:string[];baseline_fingerprint?:string;candidate_fingerprint?:string}
export type ReasoningAssessment = {material_records:number;supported_records:number;coverage_percent:number;missing_evidence:string[];dangling_evidence:string[];gate_blockers:string[]}
export type ReasoningCase = {schema_version:string;records:ReasoningRecord[];evidence:ReasoningEvidence[];experiments?:ReasoningExperiment[];assessment?:ReasoningAssessment}

type Tab = 'dash'|'sess'|'vault'|'routes'|'costs'|'settings'
const authHeaders=():Record<string,string> => apiToken() ? {Authorization:`Bearer ${apiToken()}`} : {}

async function api<T>(path:string):Promise<T> {
  const url=`${apiBase()}/api/v1${path}`
  const response = apiToken() ? await fetch(url,{headers:authHeaders()}) : await fetch(url)
  if (!response.ok) throw new Error(`API ${response.status}`)
  const payload = await response.json()
  return (payload && typeof payload === 'object' && 'data' in payload ? payload.data : payload) as T
}
async function mutate<T>(path:string,method='POST',body?:unknown):Promise<T>{
  const response=await fetch(`${apiBase()}/api/v1${path}`,{method,headers:{'Content-Type':'application/json',...authHeaders()},body:body===undefined?undefined:JSON.stringify(body)})
  const payload=await response.json().catch(()=>({}))
  if(!response.ok)throw new Error(payload?.error?.message||`API ${response.status}`)
  return (payload&&typeof payload==='object'&&'data'in payload?payload.data:payload) as T
}
async function downloadRun(runID:string,format:'markdown'|'json'){
  const response=await fetch(`${apiBase()}/api/v1/runs/${runID}/export?format=${format}`,{headers:authHeaders()})
  if(!response.ok)throw new Error(`No se pudo exportar el run (API ${response.status})`)
  const blob=await response.blob(),url=URL.createObjectURL(blob),anchor=document.createElement('a')
  anchor.href=url
  anchor.download=`oberth-run-${runID}.${format==='markdown'?'md':'json'}`
  anchor.click()
  URL.revokeObjectURL(url)
}

const tabs:{id:Tab;label:MessageKey;Icon:typeof LayoutDashboard}[] = [
  {id:'dash',label:'nav.dashboard',Icon:LayoutDashboard},
  {id:'sess',label:'nav.session',Icon:SquareTerminal},
  {id:'vault',label:'nav.vault',Icon:BookOpen},
  {id:'routes',label:'nav.routes',Icon:Route},
  {id:'costs',label:'nav.costs',Icon:Coins},
  {id:'settings',label:'nav.settings',Icon:Settings},
]
const providerColors = ['var(--p-openai)','var(--p-anthropic)','var(--p-ollama)','var(--p-google)']

export default function App() {
  const {t}=useI18n()
  const [tab,setTab] = useState<Tab>('dash')
  const [status,setStatus] = useState<Status>({})
  const [providers,setProviders] = useState<Provider[]>([])
  const [sessions,setSessions] = useState<Session[]>([])
  const [tasks,setTasks] = useState<Task[]>([])
  const [costs,setCosts] = useState<CostSummary>({})
  const [notes,setNotes] = useState<VaultNote[]>([])
  const [memoryCandidates,setMemoryCandidates] = useState<MemoryCandidate[]>([])
  const [routes,setRoutes] = useState<RouteRule[]>([])
  const [projects,setProjects] = useState<Project[]>([])
  const [runs,setRuns] = useState<Run[]>([])
  const [launchers,setLaunchers] = useState<Launcher[]>([])
  const [connected,setConnected] = useState(false)
  const [taskStreams,setTaskStreams] = useState<Record<string,string>>({})
  const [taskToOpen,setTaskToOpen] = useState('')
  const [loadFailures,setLoadFailures] = useState(0)

  const load=async()=>{
      const result=await Promise.allSettled([
        api<Status>('/status'), api<Provider[]>('/providers'), api<{sessions:Session[]}>('/sessions?limit=30'),
        api<CostSummary>('/costs'), api<VaultNote[]>('/vault/notes'), api<RouteRule[]>('/routing-rules'),api<{tasks:Task[]}>('/tasks?limit=100'),api<Project[]>('/projects'),api<Run[]>('/runs'),api<Launcher[]>('/system/launchers'),api<MemoryCandidate[]>('/memory/candidates?status=pending'),
      ])
      if(result[0].status==='fulfilled'){setStatus(result[0].value);setConnected(true)}
      else setConnected(false)
      if(result[1].status==='fulfilled')setProviders(result[1].value||[])
      if(result[2].status==='fulfilled')setSessions(result[2].value.sessions||[])
      if(result[3].status==='fulfilled')setCosts(result[3].value)
      if(result[4].status==='fulfilled')setNotes(result[4].value||[])
      if(result[5].status==='fulfilled')setRoutes(result[5].value||[])
      if(result[6].status==='fulfilled')setTasks(result[6].value.tasks||[])
      if(result[7].status==='fulfilled')setProjects(result[7].value||[])
      if(result[8].status==='fulfilled')setRuns(result[8].value||[])
      if(result[9].status==='fulfilled')setLaunchers(result[9].value||[])
      if(result[10].status==='fulfilled')setMemoryCandidates(result[10].value||[])
      const failed=result.filter(item=>item.status==='rejected').length
      setLoadFailures(failed)
    }
  useEffect(()=>{
    void load()
    const timer=window.setInterval(()=>void load(),15000)
    return()=>window.clearInterval(timer)
  },[]) // eslint-disable-line react-hooks/exhaustive-deps

  // WebSocket event subscription for live updates
  const wsRef = useRef<ReturnType<typeof useWebSocket> | null>(null)
  if (!wsRef.current) {
    wsRef.current = useWebSocket(() => setConnected(false))
  }
  useEffect(() => {
    const ws = wsRef.current!
    ws.onEvent('session.complete', () => { void load() })
    ws.onEvent('task.started', event => {
      setTaskStreams(current=>applyTaskStreamEvent(current,'task.started',event.payload))
    })
    ws.onEvent('task.chunk', event => {
      setTaskStreams(current=>applyTaskStreamEvent(current,'task.chunk',event.payload))
    })
    ws.onEvent('task.status', () => { void load() })
    ws.onEvent('vault.change', () => { void load() })
    ws.connect()
    return () => { ws.disconnect() }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const activeSession=sessions.find(s=>!['completed','approved'].includes(s.status))||sessions[0]
  const project=activeSession?.repo_path?.split(/[\\/]/).filter(Boolean).pop()||'local-workspace'
  const navigate=(next:Tab)=>{if(window.location.hash)window.history.replaceState(null,'',window.location.pathname+window.location.search);setTab(next)}

  return <div className="desktop-shell">
    <div className="titlebar"><img className="brand-mark" src="/oberth-wordmark.svg" alt="Oberth"/><span className="tsep">/</span><span className="tproj">{project}</span><span className="tver">v0.1.0-alpha.1</span></div>
    {loadFailures>0&&<div className="global-warning" role="alert">{t('load.partial',{count:loadFailures})}<button onClick={()=>void load()}>{t('common.retry')}</button></div>}
    <div className="main-frame">
      <aside className="sidebar" aria-label={t('nav.main')}>
        {tabs.map(({id,label,Icon},index)=><div key={id} className={`${index===3?'with-separator ':''}side-wrap`}><button className={`side-icon ${tab===id?'active':''}`} onClick={()=>navigate(id)} aria-label={t(label)} aria-current={tab===id?'page':undefined}><Icon size={15}/><span className="side-label">{t(label)}</span></button></div>)}
        <div className="side-bottom"><div className="side-separator"/><ProductTour onNavigate={(target:TourTarget)=>navigate(target as Tab)}/></div>
      </aside>
      <div className="content-frame">
        {tab==='dash'&&<DashboardAdmin status={status} providers={providers} costs={costs} session={activeSession} sessions={sessions} tasks={tasks} onNewTask={()=>setTab('sess')} onOpenTask={id=>{setTaskToOpen(id);setTab('sess')}}/>}
        {tab==='sess'&&<TaskWorkspace tasks={tasks} sessions={sessions} runs={runs} projects={projects} providers={providers} launchers={launchers} taskStreams={taskStreams} initialTaskID={taskToOpen} onChanged={load} onOpenSettings={()=>setTab('settings')}/>}
        {tab==='vault'&&<VaultPanel notes={notes} candidates={memoryCandidates} onChanged={load}/>}
        {tab==='routes'&&<RoutesPanel routes={routes} providers={providers} tasks={tasks} onChanged={load}/>}
        {tab==='costs'&&<CostsPanel costs={costs} sessions={sessions} providers={providers}/>} 
        {tab==='settings'&&<SettingsView providers={providers} onChanged={load}/>}
      </div>
    </div>
    <StatusBar connected={connected} status={status} providers={providers}/>
  </div>
}

function Dashboard({status,providers,costs,session}:{status:Status;providers:Provider[];costs:CostSummary;session?:Session}) {
  const{t}=useI18n()
  const live=status.server?.state==='healthy'
  const vector=vectorStorePresentation(status,t)
  const vault=status.vault?.state==='healthy'
  return <section className="panel dash">
    <Label>System</Label>
    <div className="cards3">
      <Metric label="Runtime" value={<State ok={live}>{live?'Running':'Offline'}</State>} sub="localhost:9090" hi/>
      <Metric label="Vault" value={String(status.vault?.note_count||0)} sub={`notes · ${vault?'indexed':'unavailable'}`}/>
      <Metric label={vector.label} value={<State ok={vector.ok}>{vector.value}</State>} sub={vector.detail}/>
    </div>
    <div className="cards3 secondary-cards">
      <Metric label="Cost total" value={`$${Number(costs.total_cost||0).toFixed(2)}`} sub={`${costs.total_tokens||0} tokens`}/>
      <Metric label="Session" value={session?.task_description||'No active session'} sub={session?`${session.status} · ${elapsed(session.started_at)}`:'waiting for task'} small/>
      <Metric label="Providers" value={<State ok={providers.some(p=>p.is_active)}>{providers.filter(p=>p.is_active).length} active</State>} sub={`${providers.length} configured`}/>
    </div>
    <div className="providers-block"><Label>Providers</Label>
      {providers.length?providers.map((p,i)=><div className="provider-row" key={p.id}><Dot color={providerColors[i%providerColors.length]}/><span className="provider-name">{p.name}</span><span className="provider-model">{p.default_model||providerModels(p)[0]||'auto'}</span><span className={`provider-badge ${p.is_active?'':'warn'}`}>{p.is_active?'active':'inactive'}</span><span className="provider-amount">—</span></div>):<CompactEmpty text="No providers configured"/>}
    </div>
  </section>
}

function DashboardAdmin({status,tasks,onNewTask,onOpenTask}:{status:Status;providers:Provider[];costs:CostSummary;session?:Session;sessions:Session[];tasks:Task[];onNewTask:()=>void;onOpenTask:(id:string)=>void}){
  const {t}=useI18n()
  const live=status.server?.state==='healthy'
  const attention=tasks.filter(t=>['review','blocked','failed'].includes(t.status))
  const running=tasks.filter(t=>t.status==='running')
  const failed=tasks.filter(t=>t.status==='failed')
  const priority={review:0,blocked:1,failed:2,running:3,pending:4,completed:5,cancelled:6} as Record<string,number>
  const recent=[...tasks].sort((a,b)=>(priority[a.status]??9)-(priority[b.status]??9)||new Date(b.updated_at).getTime()-new Date(a.updated_at).getTime()).slice(0,8)
  const statusLabel=(value:string)=>({pending:t('status.pending'),running:t('status.running'),review:t('status.review'),blocked:t('status.blocked'),failed:t('status.failed'),completed:t('status.completed'),cancelled:t('status.cancelled')}[value]||value)
  return <section className="panel dash">
    <header className="page-header"><div><h1>{t('dashboard.title')}</h1><p>{t('dashboard.subtitle')}</p></div><button className="primary-action" onClick={onNewTask}>{t('dashboard.newTask')}</button></header>
    {!live&&<div className="inline-notice error"><span><Dot color="var(--err)"/>{t('dashboard.serviceUnavailable')}</span><small>{t('dashboard.stale')}</small></div>}
    <div className="overview-strip" aria-label={t('dashboard.title')}><div><span>{t('dashboard.attention')}</span><strong>{attention.length}</strong></div><div><span>{t('dashboard.running')}</span><strong>{running.length}</strong></div><div><span>{t('dashboard.errors')}</span><strong>{failed.length}</strong></div></div>
    <section className="content-section"><header><div><h2>{t('dashboard.recent')}</h2><p>{recent.length?t('dashboard.openTask'):t('dashboard.noRecent')}</p></div><span>{t('dashboard.total',{count:tasks.length})}</span></header>
      {recent.length?<div className="data-list">{recent.map(task=><button type="button" className="data-row" key={task.id} aria-label={t('dashboard.openSession',{title:task.title})} onClick={()=>onOpenTask(task.id)}><i className={`task-state ${task.status}`}/><div><strong>{task.title}</strong><p>{task.description||t('dashboard.noDescription')}</p></div><span className={`status-text ${task.status}`}>{statusLabel(task.status)}</span><time>{relativeDate(task.updated_at)}</time></button>)}</div>:<CompactEmpty text={t('dashboard.empty')} detail={t('dashboard.emptyDetail')}/>}
    </section>
  </section>
}
function isLocalProvider(provider:Provider){
  if(provider.provider_type==='ollama')return true
  try{
    const host=new URL(provider.base_url||'').hostname
    return host==='localhost'||host==='127.0.0.1'||host==='::1'
  }catch{return false}
}
function providerModels(provider?:Provider){
  if(!provider)return []
  const models=Array.isArray(provider.models)?provider.models:String(provider.models||'').split(',')
  return [...new Set([provider.default_model||'',...models].map(model=>model.trim()).filter(Boolean))]
}
function roleRequiresCertifiedTools(role:WorkflowStage['role']){return role==='development'||role==='qa'}
function modelHasCertifiedTools(provider:Provider|undefined,model:string){
  return Boolean(model&&provider?.capabilities?.typed_actions_certified_models?.includes(model))
}
function HealthLine({label,ok,value}:{label:string;ok:boolean;value:string}){return <div className="health-line"><span><Dot color={ok?'var(--ok)':'var(--err)'}/>{label}</span><b className={ok?'ok':''}>{value}</b></div>}
function relativeDate(value:string){if(!value)return '—';const m=Math.max(0,Math.floor((Date.now()-new Date(value).getTime())/60000));return m<1?'now':m<60?`${m}m ago`:m<1440?`${Math.floor(m/60)}h ago`:`${Math.floor(m/1440)}d ago`}

function TaskWorkspace({tasks,sessions,runs,projects,providers,launchers,taskStreams,initialTaskID,onChanged,onOpenSettings}:{tasks:Task[];sessions:Session[];runs:Run[];projects:Project[];providers:Provider[];launchers:Launcher[];taskStreams:Record<string,string>;initialTaskID:string;onChanged:()=>Promise<void>;onOpenSettings:()=>void}){
  const [selected,setSelected]=useState<string>('')
  const [intention,setIntention]=useState('')
  const [projectID,setProjectID]=useState('')
  const [destinationMode,setDestinationMode]=useState<'closed'|'choose'|'create'|'manual'>('closed')
  const [repositoryPath,setRepositoryPath]=useState('')
  const [repositoryName,setRepositoryName]=useState('')
  const [newProjectParent,setNewProjectParent]=useState('')
  const [newProjectName,setNewProjectName]=useState('')
  const [busy,setBusy]=useState(false)
  const [error,setError]=useState('')
  const [advanced,setAdvanced]=useState(false)
  const [optionsOpen,setOptionsOpen]=useState(false)
  const [retryTaskID,setRetryTaskID]=useState('')
  const [simpleProviderID,setSimpleProviderID]=useState('')
  const [simpleModel,setSimpleModel]=useState('')
  const [workflow,setWorkflow]=useState<WorkflowStage[]>([])
  const [refreshingLocalModels,setRefreshingLocalModels]=useState(false)
  const [composerOpen,setComposerOpen]=useState(true)
  const intentionRef=useRef<HTMLTextAreaElement>(null)
  const taskDetailRef=useRef<HTMLDivElement>(null)
  const openedInitialTaskRef=useRef('')
  const refreshedLocalModels=useRef(false)
  const activeProviders=providers.filter(provider=>provider.is_active)
  const initialProvider=activeProviders[0]
  useEffect(()=>{
    if(!initialTaskID||openedInitialTaskRef.current===initialTaskID)return
    const initialTask=tasks.find(item=>item.id===initialTaskID)
    if(!initialTask?.repository_id)return
    openedInitialTaskRef.current=initialTaskID
    setProjectID(initialTask.repository_id)
    setSelected(initialTask.id)
    setComposerOpen(false)
  },[initialTaskID,tasks])
  const simpleProvider=activeProviders.find(provider=>provider.id===simpleProviderID)
  const simpleConfigured=!activeProviders.length||Boolean(simpleProvider&&simpleModel)
  const stageIsConfigured=(stage:WorkflowStage)=>Boolean(stage.provider_id&&stage.model)
  const stageIsValid=(stage:WorkflowStage)=>{
    const provider=activeProviders.find(item=>item.id===stage.provider_id)
    return Boolean(stageIsConfigured(stage)&&(!roleRequiresCertifiedTools(stage.role)||modelHasCertifiedTools(provider,stage.model)))
  }
  const workflowHasUncertifiedTools=workflow.some(stage=>roleRequiresCertifiedTools(stage.role)&&!modelHasCertifiedTools(activeProviders.find(provider=>provider.id===stage.provider_id),stage.model))
  const workflowValid=!advanced||(workflow.length>0&&workflow.every(stageIsConfigured))
  const retryWorkflowValid=workflow.length>0&&workflow.every(stageIsConfigured)
  useEffect(()=>{
    if(!simpleProvider){
      if(!simpleProviderID&&initialProvider){
        setSimpleProviderID(initialProvider.id)
        setSimpleModel(initialProvider.default_model||providerModels(initialProvider)[0]||'')
        return
      }
      setSimpleModel('')
      return
    }
    const models=providerModels(simpleProvider)
    setSimpleProviderID(simpleProvider.id)
    setSimpleModel(current=>models.includes(current)?current:simpleProvider.default_model||models[0]||'')
  },[simpleProvider?.id,simpleProvider?.default_model,simpleProviderID,initialProvider?.id,providers])
  const refreshLocalModels=async()=>{
    const local=activeProviders.filter(provider=>isLocalProvider(provider))
    if(!local.length)return
    setRefreshingLocalModels(true)
    try{await Promise.all(local.map(provider=>mutate(`/providers/${provider.id}/fetch-models`)));await onChanged()}
    catch(e){setError(e instanceof Error?`No se pudieron actualizar los modelos locales: ${e.message}`:'No se pudieron actualizar los modelos locales.')}
    finally{setRefreshingLocalModels(false)}
  }
  useEffect(()=>{
    if(refreshedLocalModels.current||!activeProviders.some(isLocalProvider))return
    refreshedLocalModels.current=true
    void refreshLocalModels()
  },[activeProviders.length])
  useEffect(()=>{
    if(!advanced||workflow.length||!initialProvider)return
    const model=initialProvider.default_model||providerModels(initialProvider)[0]||''
    setWorkflow([
      {id:'analysis',role:'analysis',provider_id:initialProvider.id,model},
      {id:'development',role:'development',provider_id:initialProvider.id,model},
      {id:'qa',role:'qa',provider_id:initialProvider.id,model},
    ])
  },[advanced,workflow.length,initialProvider])
  const task=projectID?tasks.find(t=>t.id===selected&&t.repository_id===projectID):undefined
  const configuredWorkflow=taskExecutionPlan(task)
  const taskProject=projects.find(project=>project.id===task?.repository_id)
  const session=task?[...sessions].filter(s=>s.task_id===task.id).sort((a,b)=>new Date(b.started_at).getTime()-new Date(a.started_at).getTime())[0]:undefined
  const run=[...runs].filter(item=>item.task_id===task?.id).sort((a,b)=>new Date(b.started_at||0).getTime()-new Date(a.started_at||0).getTime())[0]
  useEffect(()=>{
    if(taskDetailRef.current)taskDetailRef.current.scrollTop=0
  },[task?.id,run?.id])
  const act=async(action:'run'|'cancel')=>{
    if(!task)return
    setBusy(true);setError('')
    try{
      await mutate(`/tasks/${task.id}/${action}`)
      await onChanged()
    }catch(e){setError(e instanceof Error?e.message:String(e))}finally{setBusy(false)}
  }
  const selectedProject=projects.find(project=>project.id===projectID)
  useEffect(()=>{if(projectID&&!projects.some(project=>project.id===projectID))setProjectID('')},[projects,projectID])
  useEffect(()=>{
    if(!projectID)return
    setSelected(current=>tasks.some(item=>item.id===current&&item.repository_id===projectID)?current:tasks.find(item=>item.repository_id===projectID)?.id||'')
    setError('')
  },[projectID,tasks])
  const visibleTasks=projectID?tasks.filter(item=>item.repository_id===projectID):tasks
  const persistRepository=async(path:string,name?:string)=>{
    if(!path)return
    const fallbackName=path.replace(/[\\/]+$/,'').split(/[\\/]/).pop()||'Repositorio'
    const made=await mutate<Project>('/projects','POST',{name:name?.trim()||fallbackName,path})
    await onChanged()
    setProjectID(made.id)
    setRepositoryPath('')
    setRepositoryName('')
    setDestinationMode('closed')
  }
  const repositoryError=(value:unknown)=>{
    const detail=value instanceof Error?value.message:String(value)
    return detail.includes('path must exist and belong to a Git repository')||detail.includes('selected folder must belong to a Git repository')
      ?'La carpeta seleccionada no pertenece a un repositorio Git.'
      :detail.includes('native directory picker is unavailable')
        ?'El selector nativo no está disponible en este entorno. Podés ingresar una ruta manualmente.'
        :detail
  }
  const pickRepository=async()=>{
    setBusy(true);setError('')
    try{
      const picked=(isDesktop()?await pickNativeRepository():undefined)??await mutate<PickedRepository>('/projects/pick-directory')
      if(!picked.canceled&&picked.path)await persistRepository(picked.path,picked.name)
    }catch(e){
      setError(repositoryError(e))
    }finally{setBusy(false)}
  }
  const connectRepository=async(e:React.FormEvent)=>{
    e.preventDefault()
    setBusy(true);setError('')
    try{await persistRepository(repositoryPath.trim(),repositoryName)}
    catch(e){setError(repositoryError(e))}
    finally{setBusy(false)}
  }
  const createProject=async(e:React.FormEvent)=>{
    e.preventDefault()
    if(!newProjectParent.trim()||!newProjectName.trim())return
    setBusy(true);setError('')
    try{
      const made=await mutate<Project>('/projects/create-new','POST',{name:newProjectName.trim(),parent_path:newProjectParent.trim()})
      await onChanged()
      setProjectID(made.id)
      setNewProjectParent('')
      setNewProjectName('')
      setDestinationMode('closed')
    }catch(e){setError(e instanceof Error?e.message:String(e))}
    finally{setBusy(false)}
  }
  const pickProjectParent=async()=>{
    setBusy(true);setError('')
    try{
      const picked=(isDesktop()?await pickNativeDirectory('Elegir carpeta para el proyecto nuevo'):undefined)??await mutate<PickedRepository>('/projects/pick-parent-directory')
      if(!picked.canceled&&picked.path)setNewProjectParent(picked.path)
    }catch(e){setError(repositoryError(e))}
    finally{setBusy(false)}
  }
  const updateStage=(index:number,change:Partial<WorkflowStage>)=>setWorkflow(current=>current.map((stage,i)=>i===index?{...stage,...change}:stage))
  const prepareConfiguredRetry=()=>{
    if(!task)return
    const previousPlan=taskExecutionPlan(task)
    const editablePlan=previousPlan.length?previousPlan:initialProvider?[{
      id:'development',role:'development' as const,provider_id:initialProvider.id,
      model:initialProvider.default_model||providerModels(initialProvider)[0]||'',
    }]:[]
    setRetryTaskID(task.id)
    setAdvanced(true)
    setWorkflow(editablePlan)
    setOptionsOpen(true)
  }
  const runConfiguredRetry=async()=>{
    if(!task||task.id!==retryTaskID||!retryWorkflowValid)return
    setBusy(true);setError('')
    try{
      const constraints={execution_plan:workflow}
      await mutate(`/tasks/${task.id}`,'PUT',{constraints})
      await mutate(`/tasks/${task.id}/run`)
      setOptionsOpen(false)
      setRetryTaskID('')
      await onChanged()
    }catch(e){setError(e instanceof Error?e.message:String(e))}
    finally{setBusy(false)}
  }
  const createTask=async()=>{if(!intention.trim()||!selectedProject||!workflowValid||!simpleConfigured)return;setBusy(true);setError('');try{const text=intention.trim(),lower=text.toLowerCase();const kind=lower.includes('review')||lower.includes('revis')?'review':lower.includes('bug')||lower.includes('corrige')||lower.includes('fix')?'bug_fix':lower.includes('document')||lower.includes('readme')?'docs':lower.includes('arquitect')?'architecture':'implementation';const simplePlan=simpleProvider&&simpleModel?[{id:'development',role:'development' as const,provider_id:simpleProvider.id,model:simpleModel}]:[];const constraints=advanced?{execution_plan:workflow}:simplePlan.length?{execution_plan:simplePlan}:[];const made=await mutate<Task>('/tasks','POST',{repository_id:selectedProject.id,title:text.length>72?`${text.slice(0,69)}...`:text,description:text,task_type:kind,constraints});setSelected(made.id);setIntention('');await onChanged();try{await mutate(`/tasks/${made.id}/run`)}catch(runError){const detail=runError instanceof Error?runError.message:String(runError);setError(detail.includes('no active provider')?'La tarea quedó guardada, pero todavía no puede ejecutarse: configurá un proveedor activo.':detail)}await onChanged()}catch(e){setError(e instanceof Error?e.message:String(e));await onChanged()}finally{setBusy(false)}}
  const create=async(e:React.FormEvent)=>{e.preventDefault();void createTask()}
  const prepareNextChange=()=>{
    if(task?.repository_id)setProjectID(task.repository_id)
    setIntention('')
    setAdvanced(false)
    setComposerOpen(true)
    window.setTimeout(()=>intentionRef.current?.focus(),0)
  }
  const hasDraftRequest=Boolean(intention.trim())
  const taskRetryable=task?.status==='failed'||task?.status==='blocked'
  const taskAction=task?.status==='pending'
    ? {label:`▶ Ejecutar${taskProject?` en ${taskProject.name}`:''}`,disabled:busy,kind:'run' as const}
    : task?.status==='failed'||task?.status==='blocked'
      ? {label:`▶ Reintentar sin cambios${taskProject?` en ${taskProject.name}`:''}`,disabled:busy,kind:'run' as const}
      : task?.status==='completed'
        ? {label:`+ Nuevo cambio${taskProject?` en ${taskProject.name}`:''}`,disabled:busy,kind:'prepare' as const}
        : task?.status==='review'
          ? {label:'Esperando revisión',disabled:true,kind:'prepare' as const}
          : {label:'En ejecución',disabled:true,kind:'prepare' as const}
  return <section className="panel task-workspace">
    <section className={`request-bar ${task&&!composerOpen?'collapsed':''}`}>{task&&!composerOpen?<div className="collapsed-request"><div><strong>{task.title}</strong><span>Revisá el resultado abajo o prepará una nueva solicitud.</span></div><button type="button" onClick={()=>setComposerOpen(true)}>Nueva solicitud</button></div>:<form className="task-create" onSubmit={create}>
      <div className="compose-heading"><span>Nueva tarea</span><h1>¿Qué querés construir?</h1><p>Elegí el proyecto, describí el resultado y ejecutalo con el modelo que prefieras.</p></div>
      <div className="repository-field"><span>Proyecto</span><div className="repository-controls">{projects.length>0?<select aria-label="Repositorio" value={projectID} onChange={e=>setProjectID(e.target.value)}><option value="">Elegir proyecto</option>{projects.map(p=><option value={p.id} key={p.id}>{p.name} — {p.path}</option>)}</select>:null}<button type="button" className="repository-select-action" disabled={busy} onClick={()=>setDestinationMode('choose')}>+ Agregar proyecto</button></div></div>
      <Modal open={destinationMode!=='closed'} label="Agregar proyecto" onClose={()=>setDestinationMode('closed')} backdropClassName="destination-modal" dialogClassName="destination-card"><header><strong>{destinationMode==='create'?'Crear proyecto vacío':destinationMode==='manual'?'Conectar repositorio':'Agregar proyecto'}</strong><button type="button" aria-label="Cerrar" onClick={()=>setDestinationMode('closed')}>×</button></header>
      {destinationMode==='choose'&&<><p>Elegí dónde va a trabajar el agente.</p><button type="button" disabled={busy} onClick={()=>void pickRepository()}>Usar repositorio existente</button><button type="button" onClick={()=>setDestinationMode('create')}>Crear proyecto vacío</button><button type="button" className="repository-connect-link" onClick={()=>setDestinationMode('manual')}>Ingresar ruta manualmente</button></>}
      {destinationMode==='create'&&<div className="repository-connect new-project-form" role="group" aria-label="Crear proyecto nuevo">
        <label><span>Nombre del proyecto</span><input autoFocus aria-label="Nombre del proyecto nuevo" value={newProjectName} onChange={e=>setNewProjectName(e.target.value)} placeholder="mi-proyecto"/></label>
        <label><span>Carpeta padre</span><input aria-label="Carpeta padre del proyecto" value={newProjectParent} readOnly placeholder="Elegí una carpeta"/></label>
        <button type="button" disabled={busy} onClick={()=>void pickProjectParent()}>{busy?'Abriendo selector…':'Elegir carpeta…'}</button>
        <small>Se creará una carpeta nueva, se inicializará Git y quedará lista para la primera tarea.</small>
        <button type="button" disabled={busy||!newProjectName.trim()||!newProjectParent.trim()} onClick={e=>void createProject(e)}>{busy?'Creando proyecto…':'Crear e inicializar'}</button>
        <button type="button" className="repository-connect-link" onClick={()=>setDestinationMode('manual')}>Ingresar ruta manualmente</button>
      </div>}
      {destinationMode==='manual'&&<div className="repository-connect" role="group" aria-label="Conectar repositorio manualmente">
        <label><span>Ruta absoluta del repositorio Git</span><input autoFocus aria-label="Ruta absoluta del repositorio Git" value={repositoryPath} onChange={e=>setRepositoryPath(e.target.value)} placeholder="Ruta absoluta"/></label>
        <label><span>Nombre (opcional)</span><input aria-label="Nombre del repositorio" value={repositoryName} onChange={e=>setRepositoryName(e.target.value)} placeholder="Se obtiene de la carpeta"/></label>
        <button type="button" disabled={busy||!repositoryPath.trim()} onClick={e=>void connectRepository(e)}>{busy?'Validando repositorio…':'Conectar repositorio'}</button>
      </div>}</Modal>
      {selectedProject?<div className="repository-boundary" aria-live="polite"><strong>{selectedProject.name}</strong><code title={selectedProject.path}>{selectedProject.path}</code><span>Los cambios se harán en un worktree aislado. Tu checkout no cambia hasta que aceptes.</span></div>:<div className="repository-required" aria-live="polite">{projects.length?'Elegí explícitamente dónde trabajará el agente.':'Seleccioná un repositorio Git local para habilitar la ejecución.'}</div>}
      <div className="model-picker" aria-label="Modelo para esta solicitud">
        <label><span>Proveedor</span><select aria-label="Proveedor para esta solicitud" value={simpleProvider?.id||''} onChange={e=>{const provider=activeProviders.find(item=>item.id===e.target.value);setSimpleProviderID(e.target.value);setSimpleModel(provider?.default_model||providerModels(provider)[0]||'');setError('')}} disabled={!activeProviders.length}>{activeProviders.map(provider=><option key={provider.id} value={provider.id}>{provider.name}</option>)}</select></label>
        <label><span>Modelo</span><select aria-label="Modelo para esta solicitud" value={simpleModel} onChange={e=>{setSimpleModel(e.target.value);setError('')}} disabled={!simpleProvider||!providerModels(simpleProvider).length}>{providerModels(simpleProvider).map(model=><option key={model} value={model}>{model}</option>)}</select></label>
        <button type="button" aria-label={refreshingLocalModels?'Actualizando modelos locales':'Actualizar modelos locales'} title={refreshingLocalModels?'Actualizando modelos locales':'Actualizar modelos locales'} disabled={refreshingLocalModels||!activeProviders.some(isLocalProvider)} onClick={()=>void refreshLocalModels()}>{refreshingLocalModels?'Actualizando…':'Actualizar'}</button>
      </div>
      <label className="intention-field"><span>¿Qué querés lograr?</span><textarea ref={intentionRef} value={intention} onChange={e=>{setIntention(e.target.value);setError('')}} placeholder="Ej.: corregí la validación del login y agregá pruebas"/></label>
      <div className="request-options"><button type="button" className="request-options-trigger" onClick={()=>{setRetryTaskID('');setOptionsOpen(true);setError('')}}>Opciones avanzadas</button><Modal open={optionsOpen} label="Opciones de ejecución" onClose={()=>{setOptionsOpen(false);setRetryTaskID('')}} backdropClassName="options-modal" dialogClassName="request-options-dialog"><header><strong>{retryTaskID?'Reintentar tarea':'Opciones avanzadas'}</strong><button type="button" aria-label="Cerrar opciones" onClick={()=>{setOptionsOpen(false);setRetryTaskID('')}}>×</button></header><div className="request-options-body"><div className="request-options-title"><strong>{retryTaskID?'Elegí cómo ejecutar el nuevo intento':'Orquestación por roles'}</strong><span>{retryTaskID?'El proveedor, modelo y las etapas visibles son exactamente los que se usarán.':'Usala solo cuando quieras asignar modelos diferentes a varias etapas.'}</span></div>{retryTaskID&&task&&<div className="retry-context"><strong>{task.title}</strong><span>Intento anterior: {task.status}. Se creará un worktree limpio.</span></div>}{!retryTaskID&&<label className="workflow-toggle"><input aria-label="Orquestación por roles" type="checkbox" checked={advanced} onChange={e=>{setAdvanced(e.target.checked);if(e.target.checked)void refreshLocalModels()}}/> Usar varios roles y modelos</label>}{advanced&&!retryTaskID&&<button type="button" className="reset-workflow" onClick={()=>{setAdvanced(false);setWorkflow([])}}>Volver a un solo modelo</button>}
      {advanced&&<div className="workflow-editor" role="group" aria-label="Workflow de agentes">
        <p>Cada etapa usa el proveedor y modelo elegidos. Desarrollo modifica el worktree; las demás entregan análisis o validación.</p>{refreshingLocalModels&&<small className="model-refreshing">Actualizando modelos locales…</small>}
        {!activeProviders.length&&<div className="workflow-warning" role="alert"><strong>No hay proveedores activos.</strong><span>Configurá al menos uno para asignar modelos a los roles.</span><button type="button" onClick={onOpenSettings}>Configurar proveedores</button></div>}
        {workflow.map((stage,index)=><div className="workflow-stage" key={`${stage.id}-${index}`}>
          <select aria-label={`Rol ${index+1}`} value={stage.role} onChange={e=>updateStage(index,{role:e.target.value as WorkflowStage['role'],id:`${e.target.value}-${index+1}`})}>
            <option value="analysis">Análisis</option><option value="documentation">Documentación</option><option value="development">Desarrollo</option><option value="qa">QA</option><option value="review">Revisión</option>
          </select>
          <select aria-label={`Proveedor ${index+1}`} value={stage.provider_id} onChange={e=>{const provider=activeProviders.find(p=>p.id===e.target.value);updateStage(index,{provider_id:e.target.value,model:provider?.default_model||providerModels(provider)[0]||''})}}>
            {activeProviders.map(provider=><option key={provider.id} value={provider.id}>{provider.name}</option>)}
          </select>
          <select aria-label={`Modelo ${index+1}`} value={stage.model} onChange={e=>updateStage(index,{model:e.target.value})}>
            {providerModels(activeProviders.find(provider=>provider.id===stage.provider_id)).map(model=><option key={model} value={model}>{model}</option>)}
          </select>
          <button type="button" aria-label={`Eliminar etapa ${index+1}`} onClick={()=>setWorkflow(current=>current.filter((_,i)=>i!==index))}>×</button>
          {roleRequiresCertifiedTools(stage.role)&&!modelHasCertifiedTools(activeProviders.find(provider=>provider.id===stage.provider_id),stage.model)&&<div className="stage-certification-warning"><strong>Compatibilidad en validación:</strong> si el modelo devuelve un formato inesperado, el runtime intentará repararlo y mostrará una recuperación clara.</div>}
        </div>)}
        <button type="button" onClick={()=>{if(!initialProvider)return;setWorkflow(current=>[...current,{id:`stage-${current.length+1}`,role:'review',provider_id:initialProvider.id,model:initialProvider.default_model||providerModels(initialProvider)[0]||''}])}}>Agregar etapa</button>
        {activeProviders.length>0&&!workflow.length&&<div className="workflow-warning" role="alert">Agregá al menos una etapa o desactivá la orquestación por roles.</div>}
      </div>}{retryTaskID&&workflowHasUncertifiedTools&&<div className="retry-risk">Compatibilidad en validación. Podés continuar: el runtime intentará reparar respuestas no estructuradas.</div>}{retryTaskID&&<div className="configured-retry-actions"><button type="button" disabled={busy||!retryWorkflowValid} onClick={()=>void runConfiguredRetry()}>{busy?'Iniciando nuevo intento…':!retryWorkflowValid?'Completá el plan para continuar':'Iniciar nuevo intento'}</button><button type="button" onClick={()=>{setOptionsOpen(false);setRetryTaskID('')}}>Cancelar</button></div>}<details className="task-history"><summary>Historial de este proyecto <span>{visibleTasks.length}</span></summary><div className="task-items">{visibleTasks.map(t=><button key={t.id} className={task?.id===t.id?'active':''} onClick={()=>{setProjectID(t.repository_id||'');setSelected(t.id);setOptionsOpen(false);setRetryTaskID('');setError('')}}><span>{t.title}</span><small><i className={`task-state ${t.status}`}/>{t.status}</small></button>)}{!visibleTasks.length&&<CompactEmpty text="No hay tareas" detail="Crea la primera para comenzar."/>}</div></details></div></Modal></div>
      <button type="submit" disabled={busy||!intention.trim()||!selectedProject||!simpleConfigured||!workflowValid}>{busy?'Preparando entorno aislado…':!selectedProject?'Elegí un repositorio para continuar':!simpleConfigured?'Elegí un modelo para continuar':!workflowValid?'Completá la configuración de ejecución':`Crear y ejecutar en ${selectedProject.name}`}</button>
    </form>}
      {error&&<div className="task-error task-error-dismissible" role="alert"><span>{error}</span><button type="button" aria-label="Cerrar error" onClick={()=>setError('')}>×</button></div>}
    </section>
    <div className="task-detail" ref={taskDetailRef}>{task?<><header><div><h2>{task.title}</h2><p>{task.description||'Sin descripción adicional.'}</p></div><span className={`task-pill ${task.status}`}>{task.status}</span></header><div className="task-toolbar">{hasDraftRequest?<span className="draft-notice">Hay una solicitud nueva lista arriba. Ejecutala desde el compositor.</span>:<><button disabled={taskAction.disabled} onClick={()=>taskAction.kind==='run'?void act('run'):prepareNextChange()}>{busy?'Procesando…':taskAction.label}</button>{taskRetryable&&<button className="configure-retry" disabled={busy} onClick={prepareConfiguredRetry}>Configurar reintento</button>}</>}<button className="danger" disabled={busy||task.status!=='running'} onClick={()=>void act('cancel')}>■ Cancelar</button><span>{task.task_type} · {task.id.slice(0,8)}</span></div>{configuredWorkflow.length>0&&<details className="workflow-summary"><summary>Configuración del último intento</summary><div>{configuredWorkflow.map(stage=><span key={stage.id}><b>{stage.role}</b>{providers.find(provider=>provider.id===stage.provider_id)?.name||'provider no disponible'} · {stage.model}</span>)}</div></details>}<SessionPanel session={session} run={run} project={taskProject} launchers={launchers} stream={taskStreams[task.id]||''} onChanged={onChanged}/></>:<CompactEmpty text="Selecciona o crea una tarea"/>}</div>
  </section>
}

function taskExecutionPlan(task?:Task):WorkflowStage[]{
  if(!task?.constraints||Array.isArray(task.constraints)||typeof task.constraints!=='object')return []
  const value=(task.constraints as {execution_plan?:unknown}).execution_plan
  return Array.isArray(value)?value as WorkflowStage[]:[]
}

function SessionPanel({session,run,project,launchers,stream,onChanged}:{session?:Session;run?:Run;project?:Project;launchers:Launcher[];stream?:string;onChanged:()=>Promise<void>}) {
  const [events,setEvents]=useState<RunEvent[]>([])
  const [bundle,setBundle]=useState<Record<string,unknown>>({})
  const [runDetail,setRunDetail]=useState<Run|undefined>()
  const cursor=useRef(0)
  const [replayDegraded,setReplayDegraded]=useState(false)
  const [correction,setCorrection]=useState('')
  const [actionError,setActionError]=useState('')
  const [actionBusy,setActionBusy]=useState('')
  const [promotionReadiness,setPromotionReadiness]=useState<PromotionReadiness>()
  useEffect(()=>{cursor.current=0;setEvents([]);setBundle({});setRunDetail(undefined);if(!run)return;let stopped=false
    const replay=async()=>{try{const page=await api<{events:RunEvent[]}>(`/runs/${run.id}/events?after=${cursor.current}`);if(stopped)return;if(page.events.length){cursor.current=page.events[page.events.length-1].sequence;setEvents(current=>[...current,...page.events])}setReplayDegraded(false)}catch{if(!stopped)setReplayDegraded(true)}}
    const evidence=async()=>{try{const detail=await api<Run>(`/runs/${run.id}`);if(stopped)return;setRunDetail(detail);setBundle(detail.result_bundle||{});if(detail.state!=='review'){setPromotionReadiness(undefined);return}try{const readiness=await api<PromotionReadiness>(`/runs/${run.id}/promotion-readiness`);if(!stopped)setPromotionReadiness(readiness)}catch{if(!stopped)setPromotionReadiness({ready:false,reason:'No se pudo comprobar si el checkout está listo. Reiniciá el servicio local para actualizarlo.'})}}catch{if(!stopped)setReplayDegraded(true)}}
    void replay();void evidence();const timer=window.setInterval(()=>{void replay();void evidence()},3000);return()=>{stopped=true;window.clearInterval(timer)}
  },[run?.id])
  if(!session)return <section className="panel session-panel"><CompactEmpty text="Todavía no hay sesiones" detail="Creá una tarea para comenzar."/></section>
  const plan=Array.isArray(session.plan)?session.plan as Array<{step?:string;status?:string}>:[]
  const blocked=[...events].reverse().find(event=>event.type==='run_blocked')
  const approvalTurn=[...events].reverse().find(event=>event.type==='agent_turn'&&String((event.payload.observation as Record<string,unknown>|undefined)?.error||'').includes('approval'))
  const diff=Array.isArray(bundle.diff)?bundle.diff as DiffFile[]:[]
  const warnings=Array.isArray(bundle.warnings)?bundle.warnings.map(String):[]
  const verification=String(bundle.verification_status||'pending')
  const diffHash=String(bundle.diff_hash||'')
  const resolvedOutcome=String(bundle.outcome||'')
  const contextEvidence=(bundle.context&&typeof bundle.context==='object'?bundle.context:{}) as {metrics?:{candidate_tokens?:number;selected_tokens?:number;savings_percent?:number;selected?:number;dropped?:number};sources?:unknown[]}
  const reasoning=(bundle.reasoning&&typeof bundle.reasoning==='object'?bundle.reasoning:undefined) as ReasoningCase|undefined
  const promotable=verification==='passed'&&Boolean(diffHash)&&promotionReadiness?.ready===true
  const perform=async(label:string,action:()=>Promise<void>)=>{setActionBusy(label);setActionError('');try{await action()}catch(error){setActionError(error instanceof Error?error.message:String(error))}finally{setActionBusy('')}}
  const openIDE=async(ide:Launcher['id'],file='')=>{if(!run)return;await perform(`ide-${ide}`,async()=>{await mutate(`/runs/${run.id}/open-ide`,'POST',{ide,file,line:1})})}
  const decide=async(outcome:'accepted'|'corrected'|'rejected')=>{if(!run)return;await perform(outcome,async()=>{await mutate(`/runs/${run.id}/outcome`,'POST',{outcome});await onChanged()})}
  const requestCorrection=async()=>{if(!run||!correction.trim())return;await perform('corrected',async()=>{const task=await api<Task>(`/tasks/${run.task_id}`);await mutate(`/runs/${run.id}/outcome`,'POST',{outcome:'corrected',note:correction.trim()});await mutate(`/tasks/${run.task_id}`,'PUT',{description:`${task.description}\n\nCorrection requested after review:\n${correction.trim()}`});await mutate(`/tasks/${run.task_id}/run`,'POST',{});setCorrection('');await onChanged()})}
  const approveAndRetry=async()=>{if(!run||!approvalTurn)return;const action=approvalTurn.payload.action as {tool?:string;arguments?:Record<string,unknown>};const target=String(action?.arguments?.path||action?.arguments?.program||'');if(!target){setActionError('No se pudo identificar la operación que necesita aprobación. Revisá el timeline técnico.');return}await perform('approval',async()=>{await mutate('/approvals/resolve','POST',{idempotency_key:crypto.randomUUID(),scope:'project',decision:'allow',operation:action.tool==='command'?'command.exec':'file.write',target,user_id:'local',task_id:run.task_id,session_id:run.session_id,run_id:run.id,repository_path:runDetail?.base_repository||'',risk:'high'});await mutate(`/tasks/${run.task_id}/run`);await onChanged()})}
  return <section className="panel session-panel"><div className="session-main">
    <div className="session-head"><div className="session-title">{session.task_description||session.task_type}</div><div className="session-meta"><span>{project?.name||'Repositorio no disponible'} · {runDetail?.base_repository||run?.base_repository||project?.path||'ruta no disponible'}</span><span>base {(runDetail?.base_commit||run?.base_commit)?.slice(0,8)||'pendiente'} · worktree {runDetail?.branch||run?.branch||'pendiente'}</span><span>{elapsed(session.started_at)}</span></div><div className="repository-safety">{session.status==='completed'?'✅ Cambios aplicados al checkout principal.':'Entorno aislado · El checkout principal no cambia hasta que aceptes.'}</div></div>
      {session.status==='completed'&&<div className="completed-banner">✅ Cambios aplicados al repositorio. Escribí una nueva intención arriba y usá «Solicitar nuevo cambio» para la próxima iteración.</div>}
      {blocked&&<div className="task-error session-decision"><strong>{String(blocked.payload.code||'blocked')}</strong><div>{String(blocked.payload.cause||'La ejecución necesita atención.')}</div><small>{String(blocked.payload.next_action||'Revisa la evidencia y reintenta de forma segura.')}</small></div>}
      {approvalTurn&&<div className="task-toolbar session-decision"><button disabled={Boolean(actionBusy)} onClick={()=>void approveAndRetry()}>{actionBusy==='approval'?'Aplicando aprobación…':'Aprobar riesgo para este proyecto y reintentar'}</button></div>}
      {run?.state==='review'&&!resolvedOutcome&&<div className="review-actions session-decision"><div className="review-head"><strong>Revisá los cambios y decidí</strong><span>El checkout principal sigue protegido hasta que apliques.</span></div><div className="task-toolbar"><button disabled={Boolean(actionBusy)||!promotable} title={promotable?'QA aprobado y checkout listo para aplicar':promotionReadiness?.reason||'Comprobando que el diff y el checkout estén listos'} onClick={()=>void decide('accepted')}>{actionBusy==='accepted'?'Aplicando cambios…':promotable?`Aplicar cambios a ${project?.name||'repositorio'}`:'Aplicación bloqueada'}</button><button className="danger" disabled={Boolean(actionBusy)} onClick={()=>void decide('rejected')}>{actionBusy==='rejected'?'Descartando…':'Descartar cambios'}</button></div>{promotionReadiness?.ready===false&&<div className="promotion-readiness" role="status">{promotionReadiness.reason||'El checkout todavía no está listo para aplicar este cambio.'}</div>}<label>¿Hace falta corregir algo?<textarea value={correction} onChange={event=>setCorrection(event.target.value)} placeholder="Indicá qué debe corregir el agente; se iniciará un nuevo intento."/></label><button className="secondary-review-action" disabled={Boolean(actionBusy)||!correction.trim()} onClick={()=>void requestCorrection()}>{actionBusy==='corrected'?'Creando nuevo intento…':'Pedir corrección'}</button></div>}
      {run?.state==='review'&&resolvedOutcome&&<div className="completed-banner">Esta revisión ya fue resuelta: <strong>{resolvedOutcome}</strong>.</div>}
    <div className="session-body"><div className="session-left">
      <div className={`changes-callout ${diff.length?'has-changes':''}`}><Label>Cambios del agente</Label>{diff.length?<><strong>{diff.length} archivo(s) listos para revisar</strong><span>{diff.map(file=>file.path).join(' · ')}</span><a href="#run-diff">Ver diff completo ↓</a></>:<><strong>{run?.state==='blocked'||blocked?'Ejecución bloqueada antes de generar cambios':'Sin diff disponible'}</strong><span>{run?.state==='running'?'El diff aparecerá aquí durante la ejecución.':run?.state==='blocked'||blocked?'No hay nada para aplicar. Revisá la causa y la próxima acción en el bloque de error antes de reintentar.':'Este run no produjo cambios de archivos; revisá las advertencias y el timeline.'}</span></>}</div>
      {(runDetail?.worktree_path||run?.worktree_path)&&<div className="task-toolbar launcher-toolbar">{launchers.filter(launcher=>launcher.available).map(launcher=><button key={launcher.id} disabled={Boolean(actionBusy)} onClick={()=>void openIDE(launcher.id)}>{actionBusy===`ide-${launcher.id}`?`Abriendo ${launcher.name}…`:launcher.id==='folder'?'Abrir carpeta del worktree':`Abrir en ${launcher.name}`}</button>)}{!launchers.some(launcher=>launcher.available&&launcher.id!=='folder')&&<span>No se detectó un IDE compatible; abrí la carpeta con tu editor preferido.</span>}</div>}
      {actionError&&<div className="task-error" role="alert"><strong>No se pudo completar la acción</strong><div>{actionError}</div><small>El worktree y los cambios siguen intactos; podés reintentar.</small></div>}
      <details className="technical-details"><summary>Detalles técnicos</summary><div className="technical-details-body">
      <div><Label>Plan</Label>{plan.length?plan.map((p,i)=><div key={i} className={`plan-item ${p.status==='completed'?'done':p.status==='in_progress'?'current':''}`}><span>{p.status==='completed'?'✓':p.status==='in_progress'?'→':i+1}</span>{p.step||`Paso ${i+1}`}</div>):<div className="plan-item current"><span>→</span>Sesión en curso</div>}</div>
      <div><Label>Actividad</Label><div className="output-box"><span className="diff-head">sesión {session.id.slice(0,8)}</span>{'\n'}<span className="diff-context">estado: {session.status}</span>{'\n'}<span className="diff-add">+ modelo: {session.model||'enrutamiento automático'}</span>{'\n'}<span className="diff-add">+ tokens: {session.tokens_input+session.tokens_output}</span>{'\n'}<span className="diff-context">costo: ${session.cost.toFixed(4)}</span>{stream&&<>{'\n\n'}<span className="diff-head">salida en vivo</span>{'\n'}<span className="diff-context">{stream}</span></>}</div></div>
      <div><Label>Timeline</Label>{events.slice(-8).map(event=><div className="timeline-item" key={event.sequence}><Check size={11}/>{event.sequence}. {event.type}</div>)}{!events.length&&<div className="timeline-item running"><LoaderCircle size={11}/>Sesión {session.status}</div>}{replayDegraded&&<div className="task-error">Conexión degradada; reintentando replay desde el cursor {cursor.current}.</div>}</div>
      </div></details>
      {Object.keys(bundle).length>0&&<div className="run-evidence"><Label>Resultado</Label>
        <details className="technical-details"><summary>Ver evidencia y métricas</summary><div className="technical-details-body">
        <div className="evidence-summary"><span className={`task-pill ${verification==='passed'?'completed':'blocked'}`}>verificación: {verification}</span><span>{diffHash?`diff ligado · ${diffHash.slice(7,19)}`:'diff sin hash'}</span><span>{diff.length} archivo(s)</span><span>${Number(bundle.cost||0).toFixed(4)}</span><span>{Number(bundle.tokens_input||0)+Number(bundle.tokens_output||0)} tokens</span></div>
        {contextEvidence.metrics&&<div className="context-economy"><strong>{contextEvidence.metrics.savings_percent?`${Math.round(contextEvidence.metrics.savings_percent)}% menos contexto`:'sin reducción necesaria'}</strong><span>{contextEvidence.metrics.selected_tokens||0} de {contextEvidence.metrics.candidate_tokens||0} tokens seleccionados</span><span>{contextEvidence.metrics.selected||contextEvidence.sources?.length||0} fuentes usadas · {contextEvidence.metrics.dropped||0} descartadas</span></div>}
        {reasoning&&<ReasoningPanel value={reasoning}/>}
        {warnings.length>0&&<div className="task-error"><strong>Advertencias</strong>{warnings.map((warning,index)=><div key={index}>{warning}</div>)}</div>}
        </div></details>
        {diff.length>0?<div id="run-diff"><Label>Cambios para revisar</Label>{diff.map(file=><details className="diff-file" key={file.path} open><summary>{file.status} · {file.path}<button onClick={event=>{event.preventDefault();const preferred=launchers.find(launcher=>launcher.available&&launcher.id!=='folder')||launchers.find(launcher=>launcher.available);if(preferred)void openIDE(preferred.id,file.path)}}>Abrir archivo</button></summary><DiffPreview content={file.content}/></details>)}</div>:<CompactEmpty text="Sin cambios de archivos" detail="Puede ser una ejecución sólo de verificación."/>}
        <div className="task-toolbar"><button onClick={()=>run&&void downloadRun(run.id,'markdown')}>Descargar informe</button><button onClick={()=>run&&void downloadRun(run.id,'json')}>Exportar JSON</button></div>
        <details><summary>Ver bundle técnico completo</summary><pre className="output-box">{JSON.stringify(bundle,null,2)}</pre></details>
      </div>}
    </div></div>
  </div></section>
}

export function ReasoningPanel({value}:{value:ReasoningCase}){
  const records=Array.isArray(value.records)?value.records:[]
  const evidence=Array.isArray(value.evidence)?value.evidence:[]
  const experiments=Array.isArray(value.experiments)?value.experiments:[]
  const openUnknowns=records.filter(record=>record.kind==='unknown'&&record.status==='unresolved')
  const properties=records.filter(record=>record.kind==='property')
  const assessment=value.assessment
  const gateBlockers=assessment?.gate_blockers||[]
  const referencedEvidence=new Set(records.flatMap(record=>record.evidence_ids||[]))
  const staleEvidence=evidence.filter(item=>item.stale&&referencedEvidence.has(item.id))
  return <section className="reasoning-panel" aria-label="Razonamiento verificable">
    <div className="reasoning-head"><div><Label>Razonamiento verificable</Label><strong>{records.length} afirmaciones · {evidence.length} evidencias</strong></div><span className={`task-pill ${openUnknowns.length?'blocked':'completed'}`}>{openUnknowns.length?`${openUnknowns.length} incógnita(s)`:'sin incógnitas abiertas'}</span></div>
    {assessment&&<div className="reasoning-coverage"><span><b style={{width:`${Math.max(0,Math.min(100,assessment.coverage_percent))}%`}}/></span><strong>{Math.round(assessment.coverage_percent)}% de cobertura</strong><small>{assessment.supported_records}/{assessment.material_records} afirmaciones materiales respaldadas</small></div>}
    {gateBlockers.length>0&&<div className="reasoning-gate" role="alert"><strong>Promoción bloqueada por evidencia</strong>{gateBlockers.map(blocker=><span key={blocker}>{blocker}</span>)}</div>}
    {staleEvidence.length>0&&<div className="reasoning-gate" role="alert"><strong>Evidencia obsoleta</strong>{staleEvidence.map(item=><span key={item.id}>{item.id} · {item.source}</span>)}</div>}
    {openUnknowns.map(record=><div className="reasoning-unknown" key={record.id}><span>evidencia insuficiente</span><strong>{record.statement}</strong><small>Próxima acción: {record.next_action||'obtener la fuente faltante'}</small></div>)}
    {experiments.length>0&&<div className="reasoning-experiments"><Label>Experimentos reproducibles</Label>{experiments.map(experiment=><article className={`reasoning-experiment ${experiment.status}`} key={experiment.id}>
      <header><span>{experiment.id} · {experiment.status}</span><small>{experiment.duration_ms!=null?`${experiment.duration_ms} ms`:''}{experiment.cost!=null?` · $${experiment.cost.toFixed(4)}`:''}</small></header>
      <strong>{experiment.question}</strong><code>{experiment.command}</code>
      <div><small>Esperado</small><p>{experiment.expectation}</p><small>Observado</small><p>{experiment.observation}</p></div>
      <footer><span>{experiment.environment}</span><span>Evidencia: {experiment.evidence_ids.join(' · ')}</span>{experiment.claim_ids?.length?<span>Claims: {experiment.claim_ids.join(' · ')}</span>:null}</footer>
      {experiment.baseline_fingerprint&&experiment.candidate_fingerprint?<small className="experiment-comparison">base {experiment.baseline_fingerprint.slice(0,18)} → candidato {experiment.candidate_fingerprint.slice(0,18)}</small>:null}
    </article>)}</div>}
    <div className="reasoning-records">{records.filter(record=>record.kind!=='unknown'||record.status!=='unresolved').map(record=><div className="reasoning-record" key={record.id}><span>{record.kind} · {record.status}{record.required?' · obligatoria':''}</span><strong>{record.statement}</strong>{record.evidence_ids?.length?<small>Evidencia: {record.evidence_ids.join(' · ')}</small>:null}{record.falsifier?<small>Se refuta si: {record.falsifier}</small>:null}</div>)}</div>
    {properties.length>0&&<small className="reasoning-foot">{properties.filter(record=>record.status==='passed').length}/{properties.length} propiedades verificadas</small>}
  </section>
}

function SessionContext({session}:{session:Session}){return <aside className="session-context">
  <details><summary>Contexto técnico</summary><div className="session-context-body">
    <div><Label>Contexto</Label><div className="context-item"><span/>memoria de Vault</div><div className="context-item"><span/>estado del repositorio</div></div>
    <div><Label>Modelo</Label><div className="model-row"><span>activo</span>{session.model||'enrutamiento automático'}</div></div>
    <div><Label>Costo</Label><div className="token-copy">{session.tokens_input} entrada · {session.tokens_output} salida</div><div className="cost-bar"><i><b style={{width:`${Math.min(100,session.cost*100)}%`}}/></i><span>${session.cost.toFixed(2)}</span></div></div>
    <div><Label>Evidencia</Label><div className="context-item"><span/>worktree aislado</div><div className="context-item"><span/>registro de acciones tipadas</div><div className="context-item"><span/>bundle versionado</div></div>
  </div></details>
  </aside>}

function VaultPanel({notes,candidates,onChanged}:{notes:VaultNote[];candidates:MemoryCandidate[];onChanged:()=>Promise<void>}){
  const [folder,setFolder]=useState('architecture');const [selected,setSelected]=useState(0);const [query,setQuery]=useState('')
  const [searchResults,setSearchResults]=useState<VaultNote[]>([]);const [searchMode,setSearchMode]=useState('')
  const folders=['projects','architecture','decisions','patterns','bugs','sessions','tasks']
  const normalizedQuery=query.trim().toLowerCase()
  useEffect(()=>{if(!normalizedQuery){setSearchResults([]);setSearchMode('');return}let stopped=false;const timer=window.setTimeout(()=>{void api<VaultSearchResponse>(`/vault/search?q=${encodeURIComponent(normalizedQuery)}&limit=30`).then(response=>{if(stopped)return;setSearchResults(response.results.map(result=>({...result.note,relevance:result.score,reason:result.reason})));setSearchMode(response.metrics?.mode==='vector'?'búsqueda híbrida vectorial':response.metrics?.mode==='local-trigram'?'recuperación local · palabras + trigramas':'coincidencia exacta')}).catch(()=>{if(!stopped)setSearchMode('búsqueda no disponible')})},250);return()=>{stopped=true;window.clearTimeout(timer)}},[normalizedQuery])
  const visible=normalizedQuery?searchResults:notes.filter(n=>!folder||String(n.path||n.name||'').startsWith(folder))
  const current=visible[selected]||visible[0]
  const [candidateBusy,setCandidateBusy]=useState('')
  const decide=async(id:string,decision:'approved'|'rejected')=>{setCandidateBusy(id);try{await mutate(`/memory/candidates/${id}/decision`,'POST',{decision});await onChanged()}catch(error){setSearchMode(`No se pudo actualizar la memoria: ${error instanceof Error?error.message:String(error)}`)}finally{setCandidateBusy('')}}
  const candidateTokens=candidates.reduce((sum,item)=>sum+Math.ceil(item.content.length/4),0)
  return <section className="panel vault-panel"><aside className="vault-tree"><Label>Folders</Label>{folders.map(f=><button key={f} className={folder===f?'active':''} onClick={()=>{setFolder(f);setSelected(0)}}><Folder size={11}/>{f}<span>{notes.filter(n=>String(n.path||n.name||'').startsWith(f)).length}</span></button>)}<div className="vault-economy"><b>{notes.length}</b><span>memorias verificadas</span><b>{candidateTokens}</b><span>tokens por revisar</span></div></aside>
    <div className="vault-list"><label className="search-box"><Search size={11}/><input aria-label="Buscar en Vault" value={query} onChange={e=>{setQuery(e.target.value);setSelected(0)}} placeholder="Buscar semánticamente"/></label>{searchMode&&<div className="vault-search-mode">{searchMode}</div>}{visible.length?visible.map((n,i)=><button key={String(n.path||n.name||i)} className={selected===i?'active':''} onClick={()=>setSelected(i)} title={n.reason}><span>{String(n.path||n.name||'note.md').split('/').pop()}</span><small><i><b style={{width:`${Math.min(1,n.relevance||.75)*100}%`}}/></i>{(n.relevance||.75).toFixed(2)}</small></button>):<CompactEmpty text={normalizedQuery?'Sin resultados':'La carpeta está vacía'}/>}</div>
    <article className="vault-preview">{candidates.length?<section className="memory-candidates"><Label>Memoria propuesta · requiere aprobación</Label>{candidates.slice(0,5).map(candidate=><div className="memory-candidate" key={candidate.id}><div className="memory-provenance"><span>{candidate.kind}{candidate.claim_id?` · ${candidate.claim_id}`:''}</span><span>{candidate.validity_status||'current'}</span></div><p>{candidate.content}</p>{candidate.evidence_ids?.length?<code>Citas: {candidate.evidence_ids.join(' · ')}</code>:null}{candidate.contradicts?<small className="memory-warning">Contradice y reemplazaría {candidate.contradicts.slice(0,8)}</small>:null}<small>{Math.round(candidate.confidence*100)}% confianza · {Math.ceil(candidate.content.length/4)} tokens · commit {candidate.source_commit.slice(0,8)}</small><div><button disabled={candidateBusy===candidate.id} onClick={()=>void decide(candidate.id,'approved')}>Aprobar y guardar</button><button disabled={candidateBusy===candidate.id} onClick={()=>void decide(candidate.id,'rejected')}>Descartar</button></div></div>)}</section>:null}{current?<><h2># {String(current.path||current.name||'Nota').replace(/\.md$/,'')}</h2><div className="note-meta">Memoria local verificable · {String(current.metadata?.type||'nota')}</div><pre>{current.content||'No hay vista previa disponible.'}</pre></>:!candidates.length?<CompactEmpty text="El Vault aprenderá de ejecuciones aprobadas"/>:null}</article>
  </section>
}

function RoutesPanel({routes,providers,tasks,onChanged}:{routes:RouteRule[];providers:Provider[];tasks:Task[];onChanged:()=>Promise<void>}){
  const provider=(id?:string)=>providers.find(p=>p.id===id)
  const taskRoutes=tasks.flatMap(task=>taskExecutionPlan(task).map(stage=>({task,stage})))
  const [editing,setEditing]=useState<RouteRule|null>(null),[name,setName]=useState(''),[taskType,setTaskType]=useState(''),[repoPattern,setRepoPattern]=useState(''),[providerID,setProviderID]=useState(''),[model,setModel]=useState(''),[active,setActive]=useState(true),[message,setMessage]=useState('')
  const openEditor=(rule?:RouteRule)=>{const chosen=rule?provider(rule.provider_id):providers.find(p=>p.is_active)||providers[0];setEditing(rule||{id:''});setName(rule?.name||'');setTaskType(rule?.match_task_type||'');setRepoPattern(rule?.match_repo_pattern||'');setProviderID(rule?.provider_id||chosen?.id||'');setModel(rule?.model||chosen?.default_model||providerModels(chosen)[0]||'');setActive(rule?.is_active!==false);setMessage('')}
  const selectedProvider=provider(providerID)
  const save=async(event:React.FormEvent)=>{event.preventDefault();if(!editing||!providerID||!model.trim()||!name.trim())return;setMessage('Guardando…');const body={name:name.trim(),priority:editing.priority??routes.length+1,is_active:active,match_task_type:taskType.trim()||undefined,match_repo_pattern:repoPattern.trim()||undefined,provider_id:providerID,model:model.trim()};try{await mutate(editing.id?`/routing-rules/${editing.id}`:'/routing-rules',editing.id?'PUT':'POST',body);await onChanged();setEditing(null);setMessage('')}catch(error){setMessage(error instanceof Error?error.message:String(error))}}
  const remove=async(rule:RouteRule)=>{if(!window.confirm(`¿Eliminar la regla "${rule.name||rule.id}"?`))return;setMessage('Eliminando…');try{await mutate(`/routing-rules/${rule.id}`,'DELETE');await onChanged();setMessage('')}catch(error){setMessage(error instanceof Error?error.message:String(error))}}
  return <section className="panel routes-panel"><div className="page-header"><div><h1>Reglas globales</h1><p>Definí qué proveedor y modelo usar según el tipo de tarea o repositorio.</p></div><button className="primary-action" onClick={()=>openEditor()}>Nueva regla</button></div>
    {editing&&<form className="route-editor" onSubmit={save}><label>Nombre<input autoFocus aria-label="Nombre de regla" value={name} onChange={e=>setName(e.target.value)} required/></label><label>Tipo de tarea<input aria-label="Tipo de tarea" value={taskType} onChange={e=>setTaskType(e.target.value)} placeholder="Cualquier tipo"/></label><label>Repositorio<input aria-label="Patrón de repositorio" value={repoPattern} onChange={e=>setRepoPattern(e.target.value)} placeholder="Cualquier repositorio"/></label><label>Proveedor<select aria-label="Proveedor de regla" value={providerID} onChange={e=>{const id=e.target.value,p=provider(id);setProviderID(id);setModel(p?.default_model||providerModels(p)[0]||'')}}>{providers.map(p=><option key={p.id} value={p.id}>{p.name}</option>)}</select></label><label>Modelo<select aria-label="Modelo de regla" title={model} value={model} onChange={e=>setModel(e.target.value)}>{providerModels(selectedProvider).map(value=><option key={value}>{value}</option>)}</select></label><div className="route-editor-footer"><label className="route-active"><input type="checkbox" checked={active} onChange={e=>setActive(e.target.checked)}/> Activa</label><span className="route-editor-actions"><button type="button" onClick={()=>setEditing(null)}>Cancelar</button><button className="primary-action">{editing.id?'Guardar cambios':'Crear regla'}</button></span></div>{message&&<p role="alert">{message}</p>}</form>}
    <div className="route-columns"><span>—</span><span>coincidencia</span><span>ejecución</span></div>
    {routes.length?routes.map((r,i)=>{const p=provider(r.provider_id);return <div className="route-row" key={r.id}><span className="route-priority">{r.priority??i+1}</span><div className="route-match">{r.name||r.match_repo_pattern||`regla ${i+1}`}<small>{r.match_task_type||'todas las tareas'}</small></div><div className="route-action"><Dot color={providerColors[i%providerColors.length]}/>{p?.name||'automático'} · {r.model||p?.default_model||'predeterminado'}<button onClick={()=>openEditor(r)}>Editar</button><button className="danger" onClick={()=>void remove(r)}>Eliminar</button></div></div>}):<CompactEmpty text="No hay reglas globales" detail="Creá una regla para controlar la selección automática de proveedor y modelo."/>}
    <div className="flow-block"><Label>Rutas explícitas por tarea</Label>{taskRoutes.length?<div className="flow-card">{taskRoutes.map(({task,stage},i)=>{const p=provider(stage.provider_id);return <div key={`${task.id}-${stage.id}`}><span title={task.title}>{task.title.slice(0,36)} · {stage.role}</span><Dot color={providerColors[i%providerColors.length]}/><b>{p?.name||'provider no disponible'} · {stage.model}</b></div>})}</div>:<CompactEmpty text="No hay rutas por tarea" detail="Activá Orquestación por roles para asignar proveedores y modelos explícitos."/>}</div>
    <div className="flow-block"><Label>Fallback global efectivo</Label>{routes.some(r=>r.is_active!==false)?<div className="flow-card">{routes.filter(r=>r.is_active!==false).map((r,i)=>{const p=provider(r.provider_id);return <div key={r.id}><span>{i+1}. {r.match_task_type||r.match_repo_pattern||'cualquier tarea'}</span><Dot color={providerColors[i%providerColors.length]}/><b>{p?.name||'provider no disponible'} · {r.model||p?.default_model||'modelo por defecto'}</b></div>})}</div>:<CompactEmpty text="Resolución automática" detail="Sin una ruta explícita, el runtime elegirá el primer proveedor activo."/>}</div>
  </section>
}

function CostsPanel({costs,sessions,providers}:{costs:CostSummary;sessions:Session[];providers:Provider[]}){
  const total=Number(costs.total_cost||0);const expensive=[...sessions].sort((a,b)=>b.cost-a.cost)[0]
  const byProvider=Object.entries(costs.by_provider||{}).map(([id,value])=>[providers.find(provider=>provider.id===id)?.name||id,value] as [string,number])
  const byTask=Object.entries(sessions.reduce<Record<string,number>>((a,s)=>({...a,[s.task_type]:(a[s.task_type]||0)+s.cost}),{}))
  return <section className="panel costs-panel"><div><Label>Total</Label><div className="cost-big">${total.toFixed(2)}</div></div>
    <CostGroup label="Por proveedor" rows={byProvider.length?byProvider:providers.map(p=>[p.name,0])} total={total}/>
    <CostGroup label="Por tipo de tarea" rows={byTask} total={total}/>
    <div><Label>Sesión de mayor costo</Label>{expensive?<div className="cost-row"><span>{expensive.task_description||expensive.task_type}</span><b>${expensive.cost.toFixed(4)}</b></div>:<CompactEmpty text="Todavía no hay costos registrados"/>}</div>
  </section>
}

function SettingsView({providers,onChanged}:{providers:Provider[];onChanged:()=>Promise<void>}){
  return <main className="panel settings-view"><LanguageSettings/><SemanticSearchSettings onChanged={onChanged}/><SettingsPanel providers={providers} onChanged={onChanged}/></main>
}

function LanguageSettings(){
  const{locale,setLocale,t}=useI18n()
  return <section className="language-settings"><div><Label>{t('language.title')}</Label><p>{t('language.description')}</p></div><select aria-label={t('language.title')} value={locale} onChange={event=>setLocale(event.target.value as 'en'|'es')}><option value="en">{t('language.english')}</option><option value="es">{t('language.spanish')}</option></select></section>
}

function DiffPreview({content}:{content:string}){
  const lines=content.split('\n')
  return <pre className="output-box diff-content">{lines.map((line,index)=>{const kind=line.startsWith('+++')||line.startsWith('---')||line.startsWith('diff --git')||line.startsWith('index ')||line.startsWith('@@')?'meta':line.startsWith('+')?'add':line.startsWith('-')?'del':line.startsWith('\\')?'notice':'context';return <span className={`diff-line diff-${kind}`} key={index}>{line||' '}{index<lines.length-1?'\n':''}</span>})}</pre>
}

function SemanticSearchSettings({onChanged}:{onChanged:()=>Promise<void>}){
  const{t}=useI18n()
  const [semantic,setSemantic]=useState<SemanticSettings>(),[desired,setDesired]=useState('builtin'),[busy,setBusy]=useState(''),[message,setMessage]=useState(''),[technicalError,setTechnicalError]=useState('')
  useEffect(()=>{void api<SemanticSettings>('/semantic-search').then(value=>{setSemantic(value);setDesired(value.engine)})},[])
  const change=async(engine:string)=>{setBusy(engine);setTechnicalError('');setMessage(engine==='disabled'?t('settings.semantic.disabling'):t('settings.semantic.preparing',{engine}));try{await mutate('/semantic-search/migrate','POST',{engine});const updated=await api<SemanticSettings>('/semantic-search');setSemantic(updated);setDesired(updated.engine);setMessage(engine==='disabled'?t('settings.semantic.disabledDone'):t('settings.semantic.migrated',{engine}));await onChanged()}catch(err){const detail=err instanceof Error?err.message:String(err);setDesired(semantic?.engine||'builtin');setTechnicalError(detail);setMessage(detail.toLowerCase().includes('connection')||detail.toLowerCase().includes('conexión')?t('settings.semantic.qdrantError'):t('settings.semantic.changeError'))}finally{setBusy('')}}
  return <section className="semantic-settings-card"><div><Label>{t('settings.semantic.title')}</Label><div className="local-candidate semantic-engine-card"><div><strong><Dot color={semantic?.enabled?'var(--ok)':'var(--t3)'}/>{semantic?.enabled?t('settings.semantic.enabled'):t('settings.semantic.disabled')}<span className="provider-badge">{semantic?.engine==='builtin'?t('settings.semantic.integrated'):semantic?.engine||t('settings.semantic.loading')}</span></strong><span>{semantic?.enabled?t('settings.semantic.dimensions',{model:semantic.model||t('settings.semantic.localModel'),count:semantic.dimensions||384}):t('settings.semantic.localAvailable')}</span><code>{t('settings.semantic.cache')}</code>{desired==='qdrant'&&<small>{t('settings.semantic.destination',{url:semantic?.qdrant_url||'http://localhost:6333'})}</small>}</div><div className="semantic-engine-actions"><label htmlFor="semantic-engine">{t('settings.semantic.engine')}</label><select id="semantic-engine" value={desired} disabled={Boolean(busy)} onChange={event=>setDesired(event.target.value)}><option value="builtin">{t('settings.semantic.integrated')}</option><option value="qdrant">{t('settings.semantic.qdrant')}</option><option value="disabled">{t('settings.semantic.disabled')}</option></select><button disabled={Boolean(busy)||desired===semantic?.engine} onClick={()=>void change(desired)}>{busy?t('settings.semantic.migrating'):desired==='disabled'?t('settings.semantic.disable'):t('settings.semantic.apply')}</button></div></div>{message&&<p className={technicalError?'settings-message error':'settings-message'} role={technicalError?'alert':'status'} aria-live={technicalError?'assertive':'polite'}>{message}</p>}{technicalError&&<details className="settings-error-details"><summary>{t('settings.technicalDetails')}</summary><code>{technicalError}</code></details>}</div></section>
}

function SettingsPanel({providers,onChanged}:{providers:Provider[];onChanged:()=>Promise<void>}){
  const{t}=useI18n()
  const [kind,setKind]=useState('ollama'),[name,setName]=useState('Ollama local'),[base,setBase]=useState('http://localhost:11434'),[model,setModel]=useState('qwen2.5-coder:7b'),[key,setKey]=useState(''),[message,setMessage]=useState(''),[checking,setChecking]=useState('')
  const [editingProviderID,setEditingProviderID]=useState('')
  const [localCandidates,setLocalCandidates]=useState<LocalProviderCandidate[]>([]),[discovering,setDiscovering]=useState(false)
  const discover=async()=>{setDiscovering(true);try{setLocalCandidates(await api<LocalProviderCandidate[]>('/providers/discover-local'))}catch(err){setMessage(err instanceof Error?`No se pudo detectar el entorno local: ${err.message}`:String(err))}finally{setDiscovering(false)}}
  useEffect(()=>{void discover()},[]) // eslint-disable-line react-hooks/exhaustive-deps
  const editProvider=(provider:Provider)=>{setEditingProviderID(provider.id);setKind(provider.provider_type);setName(provider.name);setBase(provider.base_url||'');setModel(provider.default_model||providerModels(provider)[0]||'');setKey('');setMessage('')}
  const resetProviderForm=()=>{setEditingProviderID('');setKind('ollama');setName('Ollama local');setBase('http://localhost:11434');setModel('qwen2.5-coder:7b');setKey('')}
  const save=async(e:React.FormEvent)=>{e.preventDefault();setMessage('Guardando…');const body={name,provider_type:kind,base_url:base||undefined,...(key?{api_key:key}:{}),default_model:model,...(!editingProviderID?{models:model,is_active:true}:{})};try{await mutate(editingProviderID?`/providers/${editingProviderID}`:'/providers',editingProviderID?'PUT':'POST',body);setKey('');setMessage(editingProviderID?'Proveedor actualizado.':'Proveedor configurado y activo.');await onChanged();if(editingProviderID)resetProviderForm()}catch(err){setMessage(err instanceof Error?err.message:String(err))}}
  const verify=async(provider:Provider)=>{setChecking(provider.id);setMessage(`Ejecutando una inferencia mínima con ${provider.name}…`);try{const result=await mutate<{model?:string}>(`/providers/${provider.id}/test`);setMessage(`${provider.name} respondió correctamente${result.model?` con ${result.model}`:''}.`)}catch(err){setMessage(err instanceof Error?`No se pudo verificar ${provider.name}: ${err.message}`:String(err))}finally{setChecking('')}}
  const configureLocal=async(candidate:LocalProviderCandidate)=>{const defaultModel=candidate.models[0];if(!candidate.provider_type||!candidate.base_url||!defaultModel)return;setChecking(candidate.id);setMessage(`Configurando ${candidate.name}…`);try{await mutate('/providers','POST',{name:`${candidate.name} local`,provider_type:candidate.provider_type,base_url:candidate.base_url,default_model:defaultModel,models:candidate.models.join(','),is_active:true});await onChanged();setMessage(`${candidate.name} quedó configurado con ${defaultModel}.`)}catch(err){setMessage(err instanceof Error?err.message:String(err))}finally{setChecking('')}}
  const candidateMessage=(candidate:LocalProviderCandidate)=>candidate.message==='Listo para usar.'?t('settings.local.ready'):candidate.message.includes('LM Studio')?t('settings.local.startLMStudio'):candidate.message.includes('probe de versión')?t('settings.local.cliProbeFailed'):candidate.message.includes('requiere autenticación')?t('settings.local.authNeeded'):candidate.message.includes('no está instalado')?t('settings.local.cliMissing'):candidate.message
  const authStatus=(auth?:string)=>auth==='required'?t('settings.local.authRequired'):auth==='not_verified'?t('settings.local.authNotVerified'):auth==='probe_failed'?t('settings.local.authProbeFailed'):auth&&auth!=='unknown'?auth:t('settings.local.authUnknown')
  const configured=(candidate:LocalProviderCandidate)=>providers.some(provider=>(provider.base_url||'').replace(/\/$/,'')===(candidate.base_url||'').replace(/\/$/,''))
  return <section className="panel settings-panel"><div><Label>{t('settings.local.title')}</Label><div className="local-discovery-head"><span>{t('settings.local.description')}</span><button disabled={discovering} onClick={()=>void discover()}>{discovering?t('settings.local.detecting'):t('settings.local.detectAgain')}</button></div><div className="local-candidates">{localCandidates.map(candidate=><div className="local-candidate" key={candidate.id}><div><strong><Dot color={candidate.usable?'var(--ok)':candidate.installed?'var(--warn)':'var(--t3)'}/>{candidate.name}<span className="provider-badge">{candidate.kind==='agent-harness'?t('settings.local.agentHarness'):t('settings.local.inferenceProvider')}</span></strong><span>{candidateMessage(candidate)}</span>{candidate.version&&<code>{candidate.version} · {t('settings.local.auth')} {authStatus(candidate.auth)}</code>}{candidate.models.length>0&&<code>{candidate.models.slice(0,3).join(' · ')}</code>}</div>{candidate.kind==='agent-harness'?<span className={`provider-badge ${candidate.usable?'':'warn'}`}>{candidate.usable?t('settings.local.detected'):t('settings.local.unavailable')}</span>:configured(candidate)?<span className="provider-badge">{t('settings.local.configured')}</span>:candidate.usable?<button disabled={checking===candidate.id} onClick={()=>void configureLocal(candidate)}>{checking===candidate.id?t('settings.local.configuring'):t('settings.local.use')}</button>:<span className="provider-badge warn">{candidate.installed?t('settings.local.attention'):t('settings.local.notDetected')}</span>}</div>)}</div><Label>{t('settings.providers.title')}</Label>{providers.map((p,i)=><div className="provider-row" key={p.id}><Dot color={providerColors[i%providerColors.length]}/><span className="provider-name">{p.name}</span><span className="provider-model">{p.default_model}</span><span className={`provider-badge ${p.is_active?'':'warn'}`}>{p.is_active?t('settings.providers.active'):t('settings.providers.inactive')}</span><button onClick={()=>editProvider(p)}>{t('settings.providers.edit')}</button><button disabled={checking===p.id} onClick={()=>void verify(p)}>{checking===p.id?t('settings.providers.verifying'):t('settings.providers.verify')}</button></div>)}{!providers.length&&<CompactEmpty text={t('settings.providers.empty')} detail={t('settings.providers.emptyDetail')}/>} {message&&<p aria-live="polite">{message}</p>}</div><form className="provider-form" onSubmit={save}><Label>{editingProviderID?t('settings.providers.editTitle'):t('settings.providers.manual')}</Label><label>{t('settings.providers.type')}<select value={kind} onChange={e=>setKind(e.target.value)}><option value="ollama">Ollama</option><option value="openai">OpenAI</option><option value="anthropic">Anthropic</option><option value="google">Google</option><option value="custom">OpenAI-compatible custom</option></select></label><label>{t('settings.providers.name')}<input value={name} onChange={e=>setName(e.target.value)} required/></label><label>Base URL<input value={base} onChange={e=>setBase(e.target.value)}/></label><label>{t('settings.providers.model')}<input value={model} onChange={e=>setModel(e.target.value)} required/></label><label>API key<input type="password" value={key} onChange={e=>setKey(e.target.value)} autoComplete="off" placeholder={editingProviderID?t('settings.providers.keepSecret'):''}/></label><div className="provider-form-actions">{editingProviderID&&<button type="button" onClick={resetProviderForm}>{t('settings.providers.cancel')}</button>}<button>{editingProviderID?t('settings.providers.saveChanges'):t('settings.providers.save')}</button></div></form></section>
}

function CostGroup({label,rows,total}:{label:string;rows:[string,number][];total:number}){return <div><Label>{label}</Label>{rows.map(([name,value],i)=><div className="cost-row" key={name}><span><Dot color={providerColors[i%providerColors.length]}/>{name}</span><i><b style={{width:`${total?Math.min(100,value/total*100):0}%`}}/></i><strong>${Number(value).toFixed(2)}</strong></div>)}</div>}
function vectorStorePresentation(status:Status,t:ReturnType<typeof useI18n>['t']){const engine=status.vector_store?.engine||'disabled',disabled=engine==='disabled'||status.vector_store?.state==='disabled',ok=status.vector_store?.state==='healthy';if(disabled)return{label:t('statusbar.semantic'),value:t('statusbar.disabled'),detail:t('statusbar.localFallback'),ok:false};return{label:engine==='qdrant'?'Qdrant':t('statusbar.semantic'),value:ok?t('statusbar.ready'):t('statusbar.offline'),detail:engine==='qdrant'?t('statusbar.externalVector'):t('statusbar.builtinVector'),ok}}
function StatusBar({connected,status,providers}:{connected:boolean;status:Status;providers:Provider[]}){const{t}=useI18n(),vector=vectorStorePresentation(status,t);return <footer className="statusbar"><span><Dot color={connected?'var(--ok)':'var(--err)'}/>{isDesktop()?t('statusbar.desktop'):t('statusbar.web')}</span><span><Dot color={status.vault?.state==='healthy'?'var(--ok)':'var(--err)'}/>{t('statusbar.vault')} {status.vault?.state==='healthy'?t('statusbar.ok'):t('statusbar.offline')}</span><span><Dot color={vector.ok?'var(--ok)':status.vector_store?.state==='disabled'?'var(--t3)':'var(--err)'}/>{vector.label.toLowerCase()} {vector.value}</span><span className="provider-status">{providers.map((p,i)=><em key={p.id}><Dot color={providerColors[i%providerColors.length]}/>{p.name}</em>)}</span><span className="status-version">oberth v0.1.0-alpha.1</span></footer>}
function Metric({label,value,sub,hi,small}:{label:string;value:React.ReactNode;sub:string;hi?:boolean;small?:boolean}){return <div className={`metric ${hi?'hi':''}`}><span>{label}</span><strong className={small?'small':''}>{value}</strong><small>{sub}</small></div>}
function State({ok,children}:{ok:boolean;children:React.ReactNode}){return <span className={ok?'state ok':'state'}>● {children}</span>}
function Label({children}:{children:React.ReactNode}){return <div className="section-label">{children}</div>}
function Dot({color}:{color:string}){return <i className="dot" style={{background:color}}/>}
function CompactEmpty({text,detail}:{text:string;detail?:string}){return <div className="compact-empty"><FileText size={18}/><b>{text}</b>{detail&&<span>{detail}</span>}</div>}
function elapsed(value:string){const m=Math.max(0,Math.floor((Date.now()-new Date(value).getTime())/60000));return m<1?'just now':m<60?`${m}m`:`${Math.floor(m/60)}h ${m%60}m`}
