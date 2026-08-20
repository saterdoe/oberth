import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import App,{ReasoningPanel} from './App'

const ok = (data: unknown, status = 200) => Promise.resolve(new Response(JSON.stringify({ data }), {
  status,
  headers: { 'Content-Type': 'application/json' },
}))

describe('task workspace', () => {
  let projectFixtures: Array<{id:string;name:string;path:string}>
  let taskFixtures: Array<{id:string;repository_id:string;title:string;description:string;task_type:string;status:string;created_at:string;updated_at:string;constraints?:unknown}>
  let pickerResult: {canceled:boolean;name?:string;path?:string}
  let localProviderFixtures: Array<{id:string;name:string;provider_type?:string;base_url?:string;installed:boolean;running:boolean;usable:boolean;models:string[];message:string}>
  let providerFixtures: Array<{id:string;name:string;provider_type:string;is_active:boolean;default_model:string;models:string;capabilities?:{typed_actions_certified_models?:string[]}}>
  beforeEach(() => {
    projectFixtures = [{ id: 'project-1', name: 'Demo', path: 'C:\\dev\\demo' }]
    taskFixtures = []
    pickerResult = { canceled: false, name: 'nuevo', path: '/home/dev/nuevo' }
    localProviderFixtures = []
    providerFixtures = []
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/providers/discover-local')) return ok(localProviderFixtures)
      if (!init?.method && url.endsWith('/providers')) return ok(providerFixtures)
      if (init?.method === 'POST' && url.endsWith('/projects/pick-directory')) {
        return ok(pickerResult)
      }
      if (init?.method === 'POST' && url.endsWith('/projects/pick-parent-directory')) {
        return ok(pickerResult)
      }
      if (init?.method === 'POST' && url.endsWith('/projects/create-new')) {
        const body = JSON.parse(String(init.body))
        const separator = String(body.parent_path).includes('\\') ? '\\' : '/'
        const made = { id: 'project-new', name: body.name, path: `${body.parent_path}${separator}${body.name}` }
        projectFixtures = [made]
        return ok(made, 201)
      }
      if (init?.method === 'POST' && url.endsWith('/providers')) {
        return ok({ id: 'provider-local' }, 201)
      }
      if (init?.method === 'POST' && url.endsWith('/tasks')) {
        return ok({ id: 'task-new', title: 'Nueva tarea', description: 'Resultado', task_type: 'implementation', status: 'pending', created_at: new Date().toISOString(), updated_at: new Date().toISOString() }, 201)
      }
      if (init?.method === 'POST' && url.endsWith('/projects')) {
        const body = JSON.parse(String(init.body))
        if (body.path.includes('missing')) {
          return Promise.resolve(new Response(JSON.stringify({ error: { message: 'path must exist and belong to a Git repository' } }), {
            status: 400,
            headers: { 'Content-Type': 'application/json' },
          }))
        }
        const made = { id: 'project-connected', name: body.name, path: body.path }
        projectFixtures = [made]
        return ok(made, 201)
      }
      if (url.includes('/tasks?')) return ok({ tasks: taskFixtures })
      if (url.endsWith('/status')) return ok({ server: { state: 'healthy', version: '9.8.7-test' } })
      if (url.endsWith('/sessions?limit=30')) return ok({ sessions: [] })
      if (url.endsWith('/costs')) return ok({})
      if (url.endsWith('/projects')) return ok(projectFixtures)
      if (url.endsWith('/projects/project-1/code-index')) return ok({schema_version:'1',repo_id:'repo:test',indexed_files:12,chunk_count:34,last_indexed:new Date().toISOString(),fresh:true})
      if (url.includes('/projects/project-1/code-map/nodes')) return ok({schema_version:'1',repo_id:'repo:test',fingerprint:'graph:test',fresh:true,last_indexed:new Date().toISOString(),coverage:{languages:{typescript:2},node_count:2,edge_count:1},truncated:false,remaining:0,edges:[],nodes:[{id:'node:app',repo_id:'repo:test',kind:'file',label:'app.ts',path:'src/app.ts',language:'typescript',schema_version:'1'}]})
      if (url.includes('/projects/project-1/code-map/neighborhood')) return ok({schema_version:'1',repo_id:'repo:test',fingerprint:'graph:test',fresh:true,last_indexed:new Date().toISOString(),coverage:{languages:{typescript:2},node_count:2,edge_count:1},truncated:false,remaining:0,nodes:[{id:'node:app',repo_id:'repo:test',kind:'file',label:'app.ts',path:'src/app.ts',language:'typescript',schema_version:'1'},{id:'node:db',repo_id:'repo:test',kind:'file',label:'db.ts',path:'src/db.ts',language:'typescript',schema_version:'1'}],edges:[{id:'edge:1',source_id:'node:app',target_id:'node:db',kind:'imports',source_path:'src/app.ts',range:{start_line:3,end_line:3},extractor:'static-js-imports',confidence:'extracted',resolution:'resolved repository-relative import'}]})
      if (url.includes('/git/status?path=')) return ok({ files: [], branch: 'main' })
      if (url.endsWith('/git/diff')) return ok({ files: [] })
      if (url.includes('/verifier/plan?path=')) return ok({ commands: [] })
      return ok([])
    }))
  })

  afterEach(() => vi.unstubAllGlobals())

  it('opens a recent task session from Home', async () => {
    taskFixtures = [{
      id: 'task-recent', repository_id: 'project-1', title: 'Revisar autenticación', description: 'Corregir el acceso',
      task_type: 'bug_fix', status: 'review', created_at: new Date().toISOString(), updated_at: new Date().toISOString(),
    }]
    render(<App />)

    expect(await screen.findByRole('heading', { name: 'Recent tasks' })).toBeInTheDocument()
    fireEvent.click(await screen.findByRole('button', { name: 'Open session for Revisar autenticación' }))

    fireEvent.click(await screen.findByRole('button', { name: 'New request' }))
    expect(await screen.findByRole('combobox', { name: 'Repositorio' })).toHaveValue('project-1')
    expect(screen.getByRole('button', { name: 'Session' })).toHaveAttribute('aria-current', 'page')
  })

  it('supports discoverable workspace shortcuts without hijacking editors', async () => {
    render(<App />)
    await screen.findByRole('button',{name:'Dashboard'})

    fireEvent.keyDown(document,{key:'?',shiftKey:true})
    expect(screen.getByRole('dialog',{name:'Keyboard shortcuts'})).toBeInTheDocument()
    fireEvent.keyDown(document,{key:'Escape'})
    expect(screen.queryByRole('dialog',{name:'Keyboard shortcuts'})).not.toBeInTheDocument()

    fireEvent.keyDown(document,{key:'2',altKey:true})
    expect(screen.getByRole('button',{name:'Session'})).toHaveAttribute('aria-current','page')
    const intention=await screen.findByRole('textbox',{name:/Qué querés lograr/})
    fireEvent.keyDown(intention,{key:'1',altKey:true})
    expect(screen.getByRole('button',{name:'Session'})).toHaveAttribute('aria-current','page')

    fireEvent.keyDown(document,{key:'k',ctrlKey:true})
    await waitFor(()=>expect(intention).toHaveFocus())
  })

  it('loads recent tasks in descending activity order without rendering the full list initially', async () => {
    const base=Date.now()
    taskFixtures=Array.from({length:14},(_,index)=>({
      id:`task-${index}`,repository_id:'project-1',title:`Task ${index}`,description:`Description ${index}`,
      task_type:'implementation',status:'completed',created_at:new Date(base-index*1000).toISOString(),updated_at:new Date(base-index*1000).toISOString(),
    }))
    render(<App />)

    expect(await screen.findByRole('button',{name:'Open session for Task 0'})).toBeInTheDocument()
    expect(screen.queryByRole('button',{name:'Open session for Task 8'})).not.toBeInTheDocument()
    const visible=screen.getAllByRole('button',{name:/Open session for Task/})
    expect(visible).toHaveLength(8)
    expect(visible[0]).toHaveAccessibleName('Open session for Task 0')

    fireEvent.click(screen.getByRole('button',{name:'Show more (6)'}))
    expect(screen.getAllByRole('button',{name:/Open session for Task/})).toHaveLength(14)
    expect(screen.getByRole('button',{name:'Open session for Task 13'})).toBeInTheDocument()
  })

  it('creates a task from the Session workspace', async () => {
    render(<App />)
    fireEvent.click(screen.getAllByText('Session')[0])
    const execute = screen.getByRole('button', { name: 'Elegí un repositorio para continuar' })
    expect(execute).toBeDisabled()
    expect(await screen.findByText('Elegí explícitamente dónde trabajará el agente.')).toBeInTheDocument()
    fireEvent.change(screen.getByRole('combobox', { name: 'Repositorio' }), { target: { value: 'project-1' } })
    expect(screen.getByText('C:\\dev\\demo')).toBeInTheDocument()
    expect(screen.getByText(/worktree aislado/)).toBeInTheDocument()
    fireEvent.change(await screen.findByPlaceholderText('Ej.: corregí la validación del login y agregá pruebas'), { target: { value: 'Nueva tarea' } })
    fireEvent.click(screen.getByRole('button', { name: 'Crear y ejecutar en Demo' }))
    await waitFor(() => expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/tasks'),
      expect.objectContaining({ method: 'POST', body: expect.stringContaining('"repository_id":"project-1"') }),
    ))
  })

  it('never selects the first repository automatically', async () => {
    render(<App />)
    fireEvent.click(screen.getAllByText('Session')[0])
    const repository = await screen.findByRole('combobox', { name: 'Repositorio' })
    expect(repository).toHaveValue('')
    expect(screen.getByRole('button', { name: 'Elegí un repositorio para continuar' })).toBeDisabled()
  })

  it('runs against the explicitly selected repository when several exist', async () => {
    projectFixtures = [
      { id: 'project-a', name: 'API', path: 'C:\\dev\\api' },
      { id: 'project-b', name: 'Web', path: 'C:\\dev\\web' },
    ]
    render(<App />)
    fireEvent.click(screen.getAllByText('Session')[0])
    const repository = await screen.findByRole('combobox', { name: 'Repositorio' })
    expect(repository).toHaveValue('')
    fireEvent.change(repository, { target: { value: 'project-b' } })
    fireEvent.change(screen.getByPlaceholderText('Ej.: corregí la validación del login y agregá pruebas'), { target: { value: 'Actualizar Web' } })
    fireEvent.click(screen.getByRole('button', { name: 'Crear y ejecutar en Web' }))
    await waitFor(() => expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/tasks'),
      expect.objectContaining({ body: expect.stringContaining('"repository_id":"project-b"') }),
    ))
  })

  it('creates a new task from the composer instead of retrying the selected task', async () => {
    taskFixtures = [{
      id: 'task-blocked', repository_id: 'project-1', title: 'Tarea anterior', description: 'No reutilizar',
      task_type: 'implementation', status: 'blocked', created_at: new Date().toISOString(), updated_at: new Date().toISOString(),
    }]
    render(<App />)
    fireEvent.click(screen.getByRole('button', { name: 'Session' }))
    fireEvent.change(await screen.findByRole('combobox', { name: 'Repositorio' }), { target: { value: 'project-1' } })
    fireEvent.change(screen.getByPlaceholderText('Ej.: corregí la validación del login y agregá pruebas'), { target: { value: 'Crear un archivo nuevo' } })
    const createFromDraft = await screen.findByRole('button', { name: 'Crear y ejecutar en Demo' })
    fireEvent.click(createFromDraft)
    await waitFor(() => expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/tasks'),
      expect.objectContaining({ method: 'POST', body: expect.stringContaining('Crear un archivo nuevo') }),
    ))
    expect(fetch).not.toHaveBeenCalledWith(expect.stringContaining('/api/v1/tasks/task-blocked/run'), expect.anything())
  })

  it('keeps repository and Ollama model explicit across a two-project sequence', async () => {
    projectFixtures = [
      { id: 'project-a', name: 'Python app', path: 'C:\\dev\\python-app' },
      { id: 'project-b', name: 'Go app', path: 'C:\\dev\\go-app' },
    ]
    providerFixtures = [{
      id:'ollama',name:'Ollama',provider_type:'ollama',is_active:true,
      default_model:'qwen2.5-coder:1.5b',models:'qwen2.5-coder:1.5b,qwen2.5-coder:7b,gemma4:12b',
    }]
    render(<App />)
    fireEvent.click(screen.getByRole('button',{name:'Session'}))
    const repository=await screen.findByRole('combobox',{name:'Repositorio'})
    const model=await screen.findByRole('combobox',{name:'Modelo para esta solicitud'})
    const intention=screen.getByPlaceholderText('Ej.: corregí la validación del login y agregá pruebas')

    for(const request of [
      {repository:'project-a',project:'Python app',model:'qwen2.5-coder:1.5b',text:'Crea una app Python'},
      {repository:'project-b',project:'Go app',model:'qwen2.5-coder:7b',text:'Crea una app Go'},
      {repository:'project-a',project:'Python app',model:'gemma4:12b',text:'Agrega healthcheck'},
    ]){
      fireEvent.change(repository,{target:{value:request.repository}})
      fireEvent.change(model,{target:{value:request.model}})
      fireEvent.change(intention,{target:{value:request.text}})
      fireEvent.click(screen.getByRole('button',{name:`Crear y ejecutar en ${request.project}`}))
      await waitFor(()=>expect(intention).toHaveValue(''))
    }

    const requests=(fetch as ReturnType<typeof vi.fn>).mock.calls
      .filter(([url,init])=>String(url).endsWith('/tasks')&&(init as RequestInit)?.method==='POST')
      .map(([,init])=>JSON.parse(String((init as RequestInit).body)))
    expect(requests.map(body=>[body.repository_id,body.constraints.execution_plan[0].model])).toEqual([
      ['project-a','qwen2.5-coder:1.5b'],
      ['project-b','qwen2.5-coder:7b'],
      ['project-a','gemma4:12b'],
    ])
  })

  it('persists an edited workflow before retrying a failed task', async () => {
    providerFixtures = [{
      id:'ollama',name:'Ollama',provider_type:'ollama',is_active:true,default_model:'small',
      models:'small,large',capabilities:{typed_actions_certified_models:['small','large']},
    }]
    taskFixtures = [{
      id:'task-failed',repository_id:'project-1',title:'Crear TODO',description:'Crea una app TODO en C',
      task_type:'implementation',status:'failed',created_at:new Date().toISOString(),updated_at:new Date().toISOString(),
      constraints:{execution_plan:[{id:'development',role:'development',provider_id:'ollama',model:'small'}]},
    }]
    render(<App />)
    fireEvent.click(screen.getByRole('button',{name:'Session'}))
    fireEvent.change(await screen.findByRole('combobox',{name:'Repositorio'}),{target:{value:'project-1'}})
    const closeTask=screen.getByRole('button',{name:'■ Close task'})
    expect(closeTask).toBeEnabled()
    fireEvent.click(closeTask)
    await waitFor(()=>expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/tasks/task-failed/cancel'),
      expect.objectContaining({method:'POST'}),
    ))
    fireEvent.click(await screen.findByRole('button',{name:'Configure retry'}))
    expect(screen.getByText('Elegí cómo ejecutar el nuevo intento')).toBeInTheDocument()
    fireEvent.change(screen.getByRole('combobox',{name:'Modelo 1'}),{target:{value:'large'}})
    fireEvent.click(screen.getByRole('button',{name:'Agregar etapa'}))
    fireEvent.click(screen.getByRole('button',{name:'Iniciar nuevo intento'}))

    await waitFor(()=>expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/tasks/task-failed'),
      expect.objectContaining({method:'PUT',body:expect.stringContaining('"model":"large"')}),
    ))
    const updateCall=(fetch as ReturnType<typeof vi.fn>).mock.calls.find(([url,init])=>
      String(url).endsWith('/tasks/task-failed')&&(init as RequestInit)?.method==='PUT')
    const updateBody=JSON.parse(String((updateCall?.[1] as RequestInit).body))
    expect(updateBody.constraints.execution_plan).toHaveLength(2)
    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/tasks/task-failed/run'),
      expect.objectContaining({method:'POST'}),
    )
  })

  it('opens a blocked simple task with its effective model visible', async () => {
    providerFixtures = [{
      id:'ollama',name:'Ollama',provider_type:'ollama',is_active:true,default_model:'gemma:12b',
      models:'qwen:1.5b,gemma:12b',
    }]
    taskFixtures = [{
      id:'task-blocked',repository_id:'project-1',title:'Crear TODO',description:'Crea una app TODO en C',
      task_type:'implementation',status:'blocked',created_at:new Date().toISOString(),updated_at:new Date().toISOString(),
    }]
    render(<App />)
    fireEvent.click(screen.getByRole('button',{name:'Session'}))
    fireEvent.change(await screen.findByRole('combobox',{name:'Repositorio'}),{target:{value:'project-1'}})
    fireEvent.click(await screen.findByRole('button',{name:'Configure retry'}))

    expect(screen.getByRole('combobox',{name:'Rol 1'})).toHaveValue('development')
    expect(screen.getByRole('combobox',{name:'Proveedor 1'})).toHaveValue('ollama')
    expect(screen.getByRole('combobox',{name:'Modelo 1'})).toHaveValue('gemma:12b')
    expect(screen.getByText(/exactamente los que se usarán/)).toBeInTheDocument()
    expect(screen.getByRole('button',{name:'Iniciar nuevo intento'})).toBeEnabled()
  })

  it('shows an actionable empty state when no repositories exist', async () => {
    projectFixtures = []
    render(<App />)
    fireEvent.click(screen.getAllByText('Session')[0])
    fireEvent.click(await screen.findByRole('button', { name: '+ Agregar proyecto' }))
    const selectRepository = await screen.findByRole('button', { name: 'Usar repositorio existente' })
    fireEvent.click(selectRepository)
    await waitFor(() => expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/projects/pick-directory'),
      expect.objectContaining({ method: 'POST' }),
    ))
    await waitFor(() => expect(fetch).toHaveBeenCalledWith(
      expect.stringMatching(/\/api\/v1\/projects$/),
      expect.objectContaining({ method: 'POST', body: expect.stringContaining('"path":"/home/dev/nuevo"') }),
    ))
    expect(await screen.findByText('/home/dev/nuevo')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Crear y ejecutar en nuevo' })).toBeDisabled()
  })

  it('creates a project from scratch when no repository is open', async () => {
    projectFixtures = []
    render(<App />)
    fireEvent.click(screen.getAllByText('Session')[0])
    fireEvent.click(await screen.findByRole('button', { name: '+ Agregar proyecto' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Crear proyecto vacío' }))
    fireEvent.change(screen.getByRole('textbox', { name: 'Nombre del proyecto nuevo' }), { target: { value: 'greenfield' } })
    pickerResult = { canceled: false, name: 'dev', path: 'C:\\dev' }
    fireEvent.click(screen.getByRole('button', { name: 'Elegir carpeta…' }))
    expect(await screen.findByDisplayValue('C:\\dev')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Crear e inicializar' }))
    await waitFor(() => expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/projects/create-new'),
      expect.objectContaining({
        method: 'POST',
        body: expect.stringContaining('"name":"greenfield"'),
      }),
    ))
    const createCall = (fetch as unknown as {mock:{calls:[string,RequestInit][]}}).mock.calls.find(([url])=>url.includes('/projects/create-new'))
    expect(String(createCall?.[1]?.body)).not.toContain('template')
    expect(await screen.findByText('C:\\dev\\greenfield')).toBeInTheDocument()
    expect(screen.getByText(/worktree aislado/)).toBeInTheDocument()
  })

  it('closes the project dialog from its backdrop', async () => {
    render(<App />)
    fireEvent.click(screen.getAllByText('Session')[0])
    fireEvent.click(await screen.findByRole('button', { name: '+ Agregar proyecto' }))
    const dialog=await screen.findByRole('dialog',{name:'Agregar proyecto'})
    fireEvent.mouseDown(dialog.parentElement!)
    expect(screen.queryByRole('dialog',{name:'Agregar proyecto'})).not.toBeInTheDocument()
  })

  it('explains why an invalid repository cannot be connected', async () => {
    projectFixtures = []
    render(<App />)
    fireEvent.click(screen.getAllByText('Session')[0])
    fireEvent.click(await screen.findByRole('button', { name: '+ Agregar proyecto' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Crear proyecto vacío' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Ingresar ruta manualmente' }))
    fireEvent.change(screen.getByRole('textbox', { name: 'Ruta absoluta del repositorio Git' }), { target: { value: 'C:\\missing' } })
    fireEvent.click(screen.getByRole('button', { name: 'Conectar repositorio' }))
    expect(await screen.findByText('La carpeta seleccionada no pertenece a un repositorio Git.')).toBeInTheDocument()
  })

  it('keeps the workspace unchanged when native selection is canceled', async () => {
    projectFixtures = []
    pickerResult = { canceled: true }
    render(<App />)
    fireEvent.click(screen.getAllByText('Session')[0])
    fireEvent.click(await screen.findByRole('button', { name: '+ Agregar proyecto' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Usar repositorio existente' }))
    await waitFor(() => expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/projects/pick-directory'),
      expect.objectContaining({ method: 'POST' }),
    ))
    expect(screen.getByRole('button', { name: 'Elegí un repositorio para continuar' })).toBeDisabled()
    expect(screen.queryByRole('group', { name: 'Conectar repositorio manualmente' })).not.toBeInTheDocument()
  })

  it('removes the legacy Code workspace from primary navigation', async () => {
    render(<App />)
    await screen.findByRole('heading',{name:'Home'})
    expect(screen.queryByText('Code')).not.toBeInTheDocument()
  })

  it('prioritizes actionable work on the dashboard', async () => {
    taskFixtures = [
      {id:'blocked-1',repository_id:'project-1',title:'Resolver autenticación',description:'El proveedor necesita credenciales',task_type:'implementation',status:'blocked',created_at:new Date().toISOString(),updated_at:new Date().toISOString()},
      {id:'running-1',repository_id:'project-1',title:'Agregar healthcheck',description:'Implementación en curso',task_type:'implementation',status:'running',created_at:new Date().toISOString(),updated_at:new Date().toISOString()},
    ]
    render(<App />)
    expect(await screen.findByRole('heading',{name:'Home'})).toBeInTheDocument()
    expect(screen.getByText('2 total')).toBeInTheDocument()
    expect(screen.getByText('Resolver autenticación')).toBeInTheDocument()
    expect(screen.getByText('Blocked')).toBeInTheDocument()
    expect(screen.queryByText('Recent session cost')).not.toBeInTheDocument()
    expect(screen.queryByText('Local usage')).not.toBeInTheDocument()
  })

  it('detects and configures a usable local provider without manual fields', async () => {
    localProviderFixtures = [{
      id: 'ollama', name: 'Ollama', provider_type: 'ollama', base_url: 'http://127.0.0.1:11434',
      installed: true, running: true, usable: true, models: ['qwen-coder'], message: 'Listo para usar.',
    }]
    render(<App />)
    fireEvent.click(screen.getAllByText('Settings')[0])
    expect(await screen.findByText('Ready to use.')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Use in Oberth' }))
    await waitFor(() => expect(fetch).toHaveBeenCalledWith(
      expect.stringMatching(/\/api\/v1\/providers$/),
      expect.objectContaining({ method: 'POST', body: expect.stringContaining('"name":"Ollama"') }),
    ))
  })

  it('loads the runtime version and exposes repository code-index status', async () => {
    render(<App />)
    expect(await screen.findAllByText(/9\.8\.7-test/)).toHaveLength(2)
    fireEvent.click(screen.getAllByText('Settings')[0])
    expect(await screen.findByText('12 files · 34 chunks')).toBeInTheDocument()
    expect(screen.getByRole('button',{name:'Reindex'})).toBeInTheDocument()
  })

  it('reveals project code-index status in small batches', async () => {
    projectFixtures=Array.from({length:6},(_,index)=>({id:`project-${index}`,name:`Project ${index}`,path:`C:\\dev\\project-${index}`}))
    render(<App />)
    fireEvent.click(screen.getAllByText('Settings')[0])
    expect(await screen.findByText('Project 0')).toBeInTheDocument()
    expect(screen.getByText('Project 3')).toBeInTheDocument()
    expect(screen.queryByText('Project 4')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button',{name:'Show more (2)'}))
    expect(screen.getByText('Project 5')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button',{name:'Show less'}))
    expect(screen.queryByText('Project 4')).not.toBeInTheDocument()
  })

  it('explores a bounded code-map neighborhood with an equivalent table view', async () => {
    render(<App />)
    fireEvent.click(screen.getAllByText('Settings')[0])
    fireEvent.click(await screen.findByRole('button',{name:'Explore relationships'}))
    expect(await screen.findByRole('dialog',{name:'Code Map'})).toBeInTheDocument()
    fireEvent.click(await screen.findByRole('button',{name:/app\.ts/}))
    expect(await screen.findByText('db.ts')).toBeInTheDocument()
    expect(screen.getByText('src/app.ts:3')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button',{name:'Table'}))
    expect(screen.getByRole('columnheader',{name:'Relationship'})).toBeInTheDocument()
    expect(screen.getByText('imports · extracted')).toBeInTheDocument()
    expect(screen.getByText('Current index')).toBeInTheDocument()
  })

  it('clears provider credentials from the form immediately after saving', async () => {
    render(<App />)
    fireEvent.click(screen.getAllByText('Settings')[0])
    const apiKey = await screen.findByLabelText('API key')
    fireEvent.change(apiKey, { target: { value: 'secret-value' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save provider' }))
    await waitFor(() => expect(apiKey).toHaveValue(''))
  })

  it('provides a real Vault search control and does not invent an executable route', async () => {
    render(<App />)
    fireEvent.click(screen.getAllByText('Vault')[0])
    expect(await screen.findByRole('textbox', { name: 'Buscar en Vault' })).toBeInTheDocument()
    fireEvent.click(screen.getAllByText('Routes')[0])
    expect(await screen.findByText('Resolución automática')).toBeInTheDocument()
    expect(screen.queryByText('automatic routing')).not.toBeInTheDocument()
  })

  it('renders populated dashboard, vault, routes, and cost views', async () => {
    const now = new Date().toISOString()
    const data: Record<string, unknown> = {
      '/status': {server:{state:'healthy'},database:{state:'healthy'},vector_store:{state:'healthy'},vault:{state:'healthy',note_count:1}},
      '/providers': [{id:'p1',name:'Local',provider_type:'ollama',is_active:true,default_model:'coder',models:'coder'}],
      '/sessions?limit=30': {sessions:[{id:'s1',task_id:'t1',task_type:'implementation',task_description:'Build',status:'active',cost:2,tokens_input:10,tokens_output:5,started_at:now,repo_path:'C:\\repo'}]},
      '/costs': {total_cost:2,total_tokens:15,by_provider:{Local:2}},
      '/vault/notes': [{path:'architecture/system.md',content:'System note',metadata:{type:'architecture'},relevance:.9}],
      '/routing-rules': [{id:'r1',name:'Local rule',priority:1,match_task_type:'implementation',provider_id:'p1',model:'coder',is_active:true}],
      '/tasks?limit=100': {tasks:[{id:'t1',repository_id:'project-1',title:'Build it',description:'Implementation',task_type:'implementation',status:'running',created_at:now,updated_at:now,constraints:{execution_plan:[{id:'development',role:'development',provider_id:'p1',model:'coder'}]}}]},
      '/projects': projectFixtures,
      '/runs': [],
      '/system/launchers': [],
      '/memory/candidates?status=pending': [],
    }
    vi.mocked(fetch).mockImplementation((input: RequestInfo | URL) => {
      const path = String(input).replace(/^.*\/api\/v1/, '')
      return ok(data[path] ?? [])
    })
    render(<App />)
    expect(await screen.findByText('Build it')).toBeInTheDocument()

    fireEvent.click(screen.getAllByText('Vault')[0])
    expect(screen.getByText('system.md')).toBeInTheDocument()
    expect(screen.getByText('System note')).toBeInTheDocument()
    fireEvent.click(screen.getAllByText('Routes')[0])
    expect(screen.getByText('Local rule')).toBeInTheDocument()
    expect(screen.getByText(/Build it · development/)).toBeInTheDocument()
    fireEvent.click(screen.getAllByText('Costs')[0])
    expect(screen.getAllByText('$2.00').length).toBeGreaterThan(0)
    expect(screen.getByText('Build')).toBeInTheDocument()
  })

  it('serializes an explicit provider and model for every workflow role', async () => {
    providerFixtures = [
      {id:'local',name:'Ollama',provider_type:'ollama',is_active:true,default_model:'qwen-local',models:'qwen-local'},
      {id:'cloud',name:'Hugging Face',provider_type:'custom',is_active:true,default_model:'free-cloud',models:'free-cloud',capabilities:{typed_actions_certified_models:['free-cloud']}},
    ]
    render(<App />)
    fireEvent.click(screen.getAllByText('Session')[0])
    fireEvent.change(await screen.findByRole('combobox',{name:'Repositorio'}),{target:{value:'project-1'}})
    fireEvent.change(screen.getByPlaceholderText('Ej.: corregí la validación del login y agregá pruebas'),{target:{value:'Implementar con revisión'}})
    fireEvent.click(screen.getByRole('button',{name:/Opciones/}))
    fireEvent.click(screen.getByRole('checkbox',{name:'Orquestación por roles'}))
    await screen.findByRole('group',{name:'Workflow de agentes'})
    fireEvent.change(screen.getByRole('combobox',{name:'Proveedor 1'}),{target:{value:'cloud'}})
    fireEvent.change(screen.getByRole('combobox',{name:'Proveedor 2'}),{target:{value:'cloud'}})
    fireEvent.change(screen.getByRole('combobox',{name:'Proveedor 3'}),{target:{value:'cloud'}})
    fireEvent.click(screen.getByRole('button',{name:'Crear y ejecutar en Demo'}))
    await waitFor(()=>expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining('/api/v1/tasks'),
      expect.objectContaining({method:'POST',body:expect.stringContaining('"execution_plan"')}),
    ))
    const call=(fetch as ReturnType<typeof vi.fn>).mock.calls.find(([url,init])=>String(url).endsWith('/tasks')&&(init as RequestInit)?.method==='POST')
    const body=JSON.parse(String((call?.[1] as RequestInit).body))
    expect(body.constraints.execution_plan).toHaveLength(3)
    expect(body.constraints.execution_plan[0]).toMatchObject({role:'analysis',provider_id:'cloud',model:'free-cloud'})
  })

  it('blocks an invalid role workflow and takes the user to provider setup', async () => {
    render(<App />)
    fireEvent.click(screen.getAllByText('Session')[0])
    fireEvent.change(await screen.findByRole('combobox',{name:'Repositorio'}),{target:{value:'project-1'}})
    fireEvent.change(screen.getByPlaceholderText('Ej.: corregí la validación del login y agregá pruebas'),{target:{value:'Implementar autenticación'}})
    fireEvent.click(screen.getByRole('button',{name:/Opciones/}))
    fireEvent.click(screen.getByRole('checkbox',{name:'Orquestación por roles'}))

    expect(await screen.findByRole('alert')).toHaveTextContent('No hay proveedores activos')
    expect(screen.getByRole('button',{name:'Completá la configuración de ejecución'})).toBeDisabled()

    fireEvent.click(screen.getByRole('button',{name:'Configurar proveedores'}))
    expect(await screen.findByText('Manual configuration')).toBeInTheDocument()
  })

  it('warns about unverified Ollama compatibility without blocking execution', async () => {
    providerFixtures = [
      {id:'ollama',name:'Ollama',provider_type:'ollama',is_active:true,default_model:'qwen2.5-coder:7b',models:'qwen2.5-coder:7b'},
    ]
    render(<App />)
    fireEvent.click(screen.getAllByText('Session')[0])
    fireEvent.change(await screen.findByRole('combobox',{name:'Repositorio'}),{target:{value:'project-1'}})
    fireEvent.change(screen.getByPlaceholderText('Ej.: corregí la validación del login y agregá pruebas'),{target:{value:'Analizar arquitectura'}})
    fireEvent.click(screen.getByRole('button',{name:/Opciones/}))
    fireEvent.click(screen.getByRole('checkbox',{name:'Orquestación por roles'}))

    expect(await screen.findAllByText(/Compatibilidad en validación:/)).toHaveLength(2)
    expect(screen.getByRole('button',{name:'Crear y ejecutar en Demo'})).toBeEnabled()
  })
})

describe('verifiable reasoning review',()=>{
  it('shows an evidence-insufficient outcome with the exact next action',()=>{
    render(<ReasoningPanel value={{
      schema_version:'1',
      records:[
        {id:'u1',kind:'unknown',statement:'Falta la política desplegada de reintentos',status:'unresolved',next_action:'adjuntar la configuración de producción'},
        {id:'p1',kind:'property',statement:'El reproceso debe ser idempotente',status:'unknown',required:true},
      ],
      evidence:[],
      assessment:{material_records:1,supported_records:0,coverage_percent:0,missing_evidence:['p1'],dangling_evidence:[],gate_blockers:['p1: required property is not passed']},
    }}/>)
    expect(screen.getByRole('region',{name:'Razonamiento verificable'})).toBeInTheDocument()
    expect(screen.getByText('Falta la política desplegada de reintentos')).toBeInTheDocument()
    expect(screen.getByText('Próxima acción: adjuntar la configuración de producción')).toBeInTheDocument()
    expect(screen.getByText('property · unknown · obligatoria')).toBeInTheDocument()
    expect(screen.getByText('0/1 propiedades verificadas')).toBeInTheDocument()
    expect(screen.getByText('0% de cobertura')).toBeInTheDocument()
    expect(screen.getByText('Promoción bloqueada por evidencia')).toBeInTheDocument()
    expect(screen.getByText('p1: required property is not passed')).toBeInTheDocument()
  })

  it('marks stale evidence prominently',()=>{
    render(<ReasoningPanel value={{
      schema_version:'1',records:[{id:'f1',kind:'fact',statement:'main.go contiene la implementación observada',status:'supported',evidence_ids:['e1']}],
      evidence:[{id:'e1',source:'file:main.go',stale:true}],
      assessment:{material_records:0,supported_records:0,coverage_percent:0,missing_evidence:[],dangling_evidence:[],gate_blockers:[]},
    }}/>)
    expect(screen.getByText('Evidencia obsoleta')).toBeInTheDocument()
    expect(screen.getByText('e1 · file:main.go')).toBeInTheDocument()
  })

  it('renders a reproducible base-versus-candidate experiment',()=>{
    render(<ReasoningPanel value={{
      schema_version:'1',records:[],evidence:[],
      experiments:[{id:'x1',question:'¿Pasa la suite el candidato?',environment:'windows/amd64 · go1.24',command:'go test ./...',expectation:'todos los paquetes pasan',observation:'todos los paquetes pasaron',status:'passed',duration_ms:1250,cost:0.001,evidence_ids:['ev-turn-003'],claim_ids:['p-message'],baseline_fingerprint:'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',candidate_fingerprint:'sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'}],
      assessment:{material_records:0,supported_records:0,coverage_percent:0,missing_evidence:[],dangling_evidence:[],gate_blockers:[]},
    }}/>)
    expect(screen.getByText('Experimentos reproducibles')).toBeInTheDocument()
    expect(screen.getByText('¿Pasa la suite el candidato?')).toBeInTheDocument()
    expect(screen.getByText('go test ./...')).toBeInTheDocument()
    expect(screen.getByText(/base sha256:aaaaaaaaaaa/)).toBeInTheDocument()
  })
})
