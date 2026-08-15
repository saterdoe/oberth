import {createContext,ReactNode,useContext,useLayoutEffect,useMemo,useState} from 'react'
import {compatTranslations} from './i18n-compat'

export type Locale='en'|'es'
const STORAGE_KEY='oberth.locale'
const codeIndexEn={
  'codeIndex.title':'Code index','codeIndex.description':'Private hybrid retrieval by paths, symbols, text, and semantic similarity.','codeIndex.current':'current','codeIndex.stale':'stale','codeIndex.missing':'not indexed',
  'codeIndex.counts':'{files} files · {chunks} chunks','codeIndex.unavailable':'No index status is available yet.','codeIndex.lastIndexed':'Last indexed: {date}','codeIndex.indexing':'Indexing…','codeIndex.reindex':'Reindex','codeIndex.indexNow':'Index now','codeIndex.empty':'No projects configured','codeIndex.emptyDetail':'Add a project to build its code index.','codeIndex.updated':'{project}: index updated.','codeIndex.indexingProject':'Indexing {project}…',
  'vault.folders':'Folders','vault.folder.projects':'projects','vault.folder.architecture':'architecture','vault.folder.decisions':'decisions','vault.folder.patterns':'patterns','vault.folder.bugs':'bugs','vault.folder.sessions':'sessions','vault.folder.tasks':'tasks',
  'codeIndex.showMore':'Show more ({count})','codeIndex.showLess':'Show less','codeIndex.showing':'Showing {visible} of {total} projects','codeIndex.listLabel':'Project code indexes',
} as const
const codeIndexEs:{[K in keyof typeof codeIndexEn]:string}={
  'codeIndex.title':'\u00cdndice de c\u00f3digo','codeIndex.description':'Recuperaci\u00f3n h\u00edbrida privada por rutas, s\u00edmbolos, texto y similitud sem\u00e1ntica.','codeIndex.current':'actualizado','codeIndex.stale':'desactualizado','codeIndex.missing':'sin indexar',
  'codeIndex.counts':'{files} archivos · {chunks} fragmentos','codeIndex.unavailable':'Todav\u00eda no hay estado disponible.','codeIndex.lastIndexed':'\u00daltima indexaci\u00f3n: {date}','codeIndex.indexing':'Indexando…','codeIndex.reindex':'Reindexar','codeIndex.indexNow':'Indexar ahora','codeIndex.empty':'No hay proyectos configurados','codeIndex.emptyDetail':'Agreg\u00e1 un proyecto para construir su \u00edndice de c\u00f3digo.','codeIndex.updated':'{project}: \u00edndice actualizado.','codeIndex.indexingProject':'Indexando {project}…',
  'vault.folders':'Carpetas','vault.folder.projects':'proyectos','vault.folder.architecture':'arquitectura','vault.folder.decisions':'decisiones','vault.folder.patterns':'patrones','vault.folder.bugs':'errores','vault.folder.sessions':'sesiones','vault.folder.tasks':'tareas',
  'codeIndex.showMore':'Mostrar más ({count})','codeIndex.showLess':'Mostrar menos','codeIndex.showing':'Mostrando {visible} de {total} proyectos','codeIndex.listLabel':'Índices de código por proyecto',
}
const sessionEn={
  'dashboard.showMore':'Show more ({count})',
  'session.run':'Run{project}',
  'session.checkResult':'Check the result below or prepare a new request.','session.newRequest':'New request','session.processing':'Processing…','session.retryNoChanges':'Retry without changes{project}','session.configureRetry':'Configure retry','session.newChange':'New change{project}','session.waitingReview':'Waiting for review','session.running':'Running','session.cancelRun':'Cancel run','session.closeTask':'Close task','session.noDescription':'No additional description.','session.lastAttempt':'Last attempt configuration',
  'session.repoUnavailable':'Repository unavailable','session.pathUnavailable':'path unavailable','session.basePending':'pending','session.worktreePending':'pending','session.readOnly':'Read-only query · The repository was not modified.','session.appliedCheckout':'Changes applied to the main checkout.','session.isolated':'Isolated environment · The main checkout does not change until you accept.','session.appliedRepository':'Changes applied to the repository. Write a new intent above and use “Request new change” for the next iteration.',
  'session.agentChanges':'Agent changes','session.filesReady':'{count} file(s) ready to review','session.viewDiff':'See full diff ↓','session.openWorktree':'Open worktree folder','session.openIn':'Open in {ide}','session.opening':'Opening {ide}…','session.noIDE':'No compatible IDE was detected; open the folder with your preferred editor.',
  'conversation.title':'Conversation','conversation.you':'You','session.technicalDetails':'Technical details','session.plan':'Plan','session.activity':'Activity','session.timeline':'Timeline','session.current':'Session in progress','session.result':'Result',
} as const
const sessionEs:{[K in keyof typeof sessionEn]:string}={
  'dashboard.showMore':'Mostrar más ({count})',
  'session.run':'Ejecutar{project}',
  'session.checkResult':'Revisá el resultado abajo o prepará una nueva solicitud.','session.newRequest':'Nueva solicitud','session.processing':'Procesando…','session.retryNoChanges':'Reintentar sin cambios{project}','session.configureRetry':'Configurar reintento','session.newChange':'Nuevo cambio{project}','session.waitingReview':'Esperando revisión','session.running':'En ejecución','session.cancelRun':'Cancelar ejecución','session.closeTask':'Cerrar tarea','session.noDescription':'Sin descripción adicional.','session.lastAttempt':'Configuración del último intento',
  'session.repoUnavailable':'Repositorio no disponible','session.pathUnavailable':'ruta no disponible','session.basePending':'pendiente','session.worktreePending':'pendiente','session.readOnly':'Consulta de sólo lectura · No se modificó el repositorio.','session.appliedCheckout':'Cambios aplicados al checkout principal.','session.isolated':'Entorno aislado · El checkout principal no cambia hasta que aceptes.','session.appliedRepository':'Cambios aplicados al repositorio. Escribí una nueva intención arriba y usá «Solicitar nuevo cambio» para la próxima iteración.',
  'session.agentChanges':'Cambios del agente','session.filesReady':'{count} archivo(s) listos para revisar','session.viewDiff':'Ver diff completo ↓','session.openWorktree':'Abrir carpeta del worktree','session.openIn':'Abrir en {ide}','session.opening':'Abriendo {ide}…','session.noIDE':'No se detectó un IDE compatible; abrí la carpeta con tu editor preferido.',
  'conversation.title':'Conversación','conversation.you':'Vos','session.technicalDetails':'Detalles técnicos','session.plan':'Plan','session.activity':'Actividad','session.timeline':'Línea de tiempo','session.current':'Sesión en curso','session.result':'Resultado',
}
const en={
  ...codeIndexEn,
  ...sessionEn,
  'nav.dashboard':'Dashboard','nav.session':'Session','nav.vault':'Vault','nav.routes':'Routes','nav.costs':'Costs','nav.settings':'Settings','nav.main':'Main navigation',
  'shortcuts.open':'Open keyboard shortcuts','shortcuts.help':'Shortcuts','shortcuts.title':'Keyboard shortcuts','shortcuts.close':'Close keyboard shortcuts','shortcuts.description':'Move through Oberth without leaving the keyboard. Shortcuts are paused while you type.','shortcuts.navigate':'Navigate to a workspace','shortcuts.compose':'Open Session and focus the request','shortcuts.show':'Show this help','shortcuts.dismiss':'Close a dialog',
  'dashboard.title':'Home','dashboard.subtitle':'Resume work that needs a decision.','dashboard.newTask':'New task',
  'dashboard.serviceUnavailable':'The local service is unavailable.','dashboard.stale':'The information may be out of date.',
  'dashboard.attention':'Needs attention','dashboard.running':'In progress','dashboard.errors':'With errors','dashboard.recent':'Recent tasks',
  'dashboard.openTask':'Open a task to review its session and continue.','dashboard.noRecent':'Your latest tasks will appear here.',
  'dashboard.total':'{count} total','dashboard.openSession':'Open session for {title}','dashboard.noDescription':'No description',
  'dashboard.empty':'No tasks yet','dashboard.emptyDetail':'Create a task to get started.',
  'status.pending':'Pending','status.running':'In progress','status.review':'Ready for review','status.blocked':'Blocked','status.failed':'Failed','status.completed':'Completed','status.cancelled':'Cancelled',
  'common.retry':'Retry','load.partial':'Could not update {count} sections. The latest available data is preserved.',
  'language.title':'Language','language.description':'Choose the language used by Oberth on this device.','language.english':'English','language.spanish':'Spanish',
  'startup.preparing':'Preparing the local service…','startup.detail':'Starting the database and checking the environment.','startup.failed':'Oberth could not start','startup.retrying':'Retrying…','startup.noResponse':'The local service did not respond.',
  'tour.open':'Open product tour','tour.title':'Product tour','tour.close':'Close tour','tour.step':'Step {current} of {total}','tour.skip':'Skip tour','tour.back':'Back','tour.next':'Next','tour.finish':'Finish',
  'tour.work.title':'One place to follow the work','tour.work.body':'Home shows work that needs attention and recent tasks. From here you can resume a run or start a new one.','tour.work.hint':'The sidebar separates work, memory and configuration.',
  'tour.model.title':'First, connect a model','tour.model.body':'In Settings you can detect Ollama and other local providers. Newly downloaded models appear after detecting or refreshing again.','tour.model.hint':'A local provider keeps code on your computer and does not consume cloud credits.',
  'tour.task.title':'Ask for a concrete outcome','tour.task.body':'In Session, choose the project and model, then describe what must be completed. You can also create a project from scratch.','tour.task.hint':'The model decides the language and structure from your instruction; Oberth does not assume them.',
  'tour.review.title':'Review before applying','tour.review.body':'Every task works in isolation. When it finishes, you will see changes, checks and warnings before deciding whether to apply them.','tour.review.hint':'Changing the project or model creates a clear and traceable work context.',
  'tour.memory.title':'Memory improves with your judgment','tour.memory.body':'Vault keeps only the useful context you approve, so future tasks need less history while preserving important decisions.','tour.memory.hint':'You can reopen this tour from the help button in the sidebar.',
  'settings.semantic.title':'Semantic search','settings.semantic.enabled':'Enabled','settings.semantic.disabled':'Disabled','settings.semantic.integrated':'built-in','settings.semantic.loading':'loading','settings.semantic.localModel':'local model',
  'settings.semantic.dimensions':'{model} · {count} dimensions','settings.semantic.localAvailable':'Word and trigram search remains available.','settings.semantic.cache':'Embeddings are preserved in a portable cache across engines.',
  'settings.semantic.destination':'Destination: {url} · a new verifiable collection will be created.','settings.semantic.engine':'Vector engine','settings.semantic.qdrant':'Qdrant','settings.semantic.migrating':'Migrating…','settings.semantic.disable':'Disable','settings.semantic.apply':'Apply change',
  'settings.semantic.disabling':'Disabling semantic search…','settings.semantic.preparing':'Preparing {engine} index…','settings.semantic.disabledDone':'Semantic search disabled; local search remains available.','settings.semantic.migrated':'Migration to {engine} completed and verified.',
  'settings.semantic.qdrantError':'Could not connect to Qdrant. Search continues with the built-in engine and no information was lost.','settings.semantic.changeError':'The engine could not be changed. Search continues with the previous configuration.','settings.technicalDetails':'Technical details',
  'settings.local.title':'Local environment','settings.local.description':'Providers generate inferences; agent harnesses explore, edit and verify repositories.','settings.local.detecting':'Detecting…','settings.local.detectAgain':'Detect again',
  'settings.local.discoveryError':'Could not detect the local environment: {detail}',
  'settings.local.detected':'detected','settings.local.unavailable':'unavailable','settings.local.configured':'configured','settings.local.configuring':'Configuring…','settings.local.use':'Use in Oberth','settings.local.attention':'needs attention','settings.local.notDetected':'not detected',
  'settings.local.inferenceProvider':'inference provider','settings.local.agentHarness':'agent harness','settings.local.ready':'Ready to use.','settings.local.startLMStudio':'Start the local server and load a model from LM Studio.','settings.local.cliProbeFailed':'CLI detected, but its version probe failed; it cannot be run from Oberth yet.','settings.local.authNeeded':'Installed, but authentication is required.','settings.local.cliMissing':'CLI is not installed or is not available in PATH.',
  'settings.local.auth':'authentication','settings.local.authUnknown':'unknown','settings.local.authRequired':'required','settings.local.authNotVerified':'not verified','settings.local.authProbeFailed':'check failed',
  'settings.providers.title':'LLM providers','settings.providers.active':'active','settings.providers.inactive':'inactive','settings.providers.edit':'Edit','settings.providers.verify':'Verify','settings.providers.verifying':'Verifying…',
  'settings.providers.empty':'No providers configured','settings.providers.emptyDetail':'Enable a detected provider above or configure a service manually.','settings.providers.editTitle':'Edit provider','settings.providers.manual':'Manual configuration',
  'settings.providers.type':'Type','settings.providers.name':'Name','settings.providers.model':'Model','settings.providers.cancel':'Cancel','settings.providers.saveChanges':'Save changes','settings.providers.save':'Save provider','settings.providers.keepSecret':'Leave blank to keep the current key',
  'statusbar.desktop':'local app','statusbar.web':'local server','statusbar.vault':'vault','statusbar.ok':'ok','statusbar.offline':'offline','statusbar.semantic':'semantic search','statusbar.disabled':'disabled','statusbar.localFallback':'local fallback available','statusbar.ready':'ready','statusbar.externalVector':'external vector store','statusbar.builtinVector':'built-in vector index',
} as const
type Messages={ [K in keyof typeof en]:string }
const es:Messages={
  ...codeIndexEs,
  ...sessionEs,
  'nav.dashboard':'Inicio','nav.session':'Sesión','nav.vault':'Memoria','nav.routes':'Rutas','nav.costs':'Costos','nav.settings':'Configuración','nav.main':'Navegación principal',
  'shortcuts.open':'Abrir atajos de teclado','shortcuts.help':'Atajos','shortcuts.title':'Atajos de teclado','shortcuts.close':'Cerrar atajos de teclado','shortcuts.description':'Recorré Oberth sin soltar el teclado. Los atajos se pausan mientras escribís.','shortcuts.navigate':'Ir a un espacio de trabajo','shortcuts.compose':'Abrir Sesión y enfocar la solicitud','shortcuts.show':'Mostrar esta ayuda','shortcuts.dismiss':'Cerrar un diálogo',
  'dashboard.title':'Inicio','dashboard.subtitle':'Retomá el trabajo que necesita una decisión.','dashboard.newTask':'Nueva tarea',
  'dashboard.serviceUnavailable':'El servicio local no está disponible.','dashboard.stale':'La información puede estar desactualizada.',
  'dashboard.attention':'Requieren atención','dashboard.running':'En curso','dashboard.errors':'Con error','dashboard.recent':'Tareas recientes',
  'dashboard.openTask':'Abrí una tarea para ver su sesión y continuar.','dashboard.noRecent':'Tus últimas tareas aparecerán acá.',
  'dashboard.total':'{count} en total','dashboard.openSession':'Abrir sesión de {title}','dashboard.noDescription':'Sin descripción',
  'dashboard.empty':'Todavía no hay tareas','dashboard.emptyDetail':'Creá una tarea para empezar.',
  'status.pending':'Pendiente','status.running':'En curso','status.review':'Para revisar','status.blocked':'Bloqueada','status.failed':'Falló','status.completed':'Completada','status.cancelled':'Cancelada',
  'common.retry':'Reintentar','load.partial':'No se pudieron actualizar {count} secciones. Se conservan los últimos datos disponibles.',
  'language.title':'Idioma','language.description':'Elegí el idioma que Oberth usará en este dispositivo.','language.english':'Inglés','language.spanish':'Español',
  'startup.preparing':'Preparando el servicio local…','startup.detail':'Estamos iniciando la base de datos y verificando el entorno.','startup.failed':'No se pudo iniciar Oberth','startup.retrying':'Reintentando…','startup.noResponse':'El servicio local no respondió.',
  'tour.open':'Abrir paseo del producto','tour.title':'Paseo del producto','tour.close':'Cerrar paseo','tour.step':'Paso {current} de {total}','tour.skip':'Omitir paseo','tour.back':'Atrás','tour.next':'Siguiente','tour.finish':'Finalizar',
  'tour.work.title':'Un lugar para seguir el trabajo','tour.work.body':'Inicio muestra lo que requiere atención y las tareas recientes. Desde acá podés retomar una ejecución o comenzar una nueva.','tour.work.hint':'La navegación lateral separa trabajo, memoria y configuración.',
  'tour.model.title':'Primero, conectá un modelo','tour.model.body':'En Configuración podés detectar Ollama y otros proveedores locales. Los modelos recién descargados aparecen al volver a detectar o actualizar.','tour.model.hint':'Un proveedor local mantiene el código en tu equipo y no consume créditos en la nube.',
  'tour.task.title':'Pedí un resultado concreto','tour.task.body':'En Sesión elegí el proyecto, el modelo y describí qué debe quedar terminado. Si el proyecto todavía no existe, también podés crearlo desde cero.','tour.task.hint':'El lenguaje y la estructura los decide el modelo a partir de tu instrucción; Oberth no los presupone.',
  'tour.review.title':'Revisá antes de aplicar','tour.review.body':'Cada tarea trabaja de forma aislada. Al terminar vas a ver cambios, verificaciones y advertencias antes de decidir si los aplicás al proyecto.','tour.review.hint':'Cambiar de proyecto o de modelo crea un contexto de trabajo claro y trazable.',
  'tour.memory.title':'La memoria mejora con tu criterio','tour.memory.body':'Vault conserva únicamente el contexto útil que aprobás, para que las próximas tareas necesiten menos historial y mantengan decisiones importantes.','tour.memory.hint':'Podés volver a abrir este paseo desde el botón de ayuda de la barra lateral.',
  'settings.semantic.title':'Búsqueda semántica','settings.semantic.enabled':'Activada','settings.semantic.disabled':'Desactivada','settings.semantic.integrated':'integrado','settings.semantic.loading':'cargando','settings.semantic.localModel':'modelo local',
  'settings.semantic.dimensions':'{model} · {count} dimensiones','settings.semantic.localAvailable':'La búsqueda por palabras y trigramas continúa disponible.','settings.semantic.cache':'Las representaciones vectoriales se conservan en una caché portable entre motores.',
  'settings.semantic.destination':'Destino: {url} · se creará una colección nueva y verificable.','settings.semantic.engine':'Motor vectorial','settings.semantic.qdrant':'Qdrant','settings.semantic.migrating':'Migrando…','settings.semantic.disable':'Desactivar','settings.semantic.apply':'Aplicar cambio',
  'settings.semantic.disabling':'Desactivando búsqueda semántica…','settings.semantic.preparing':'Preparando índice {engine}…','settings.semantic.disabledDone':'Búsqueda semántica desactivada; la búsqueda local sigue disponible.','settings.semantic.migrated':'Migración a {engine} completada y verificada.',
  'settings.semantic.qdrantError':'No pudimos conectar con Qdrant. La búsqueda sigue usando el motor Integrado y no se perdió información.','settings.semantic.changeError':'No se pudo cambiar el motor. La búsqueda continúa usando la configuración anterior.','settings.technicalDetails':'Detalles técnicos',
  'settings.local.title':'Entorno local','settings.local.description':'Los proveedores generan inferencias; los agentes exploran, editan y verifican repositorios.','settings.local.detecting':'Detectando…','settings.local.detectAgain':'Volver a detectar',
  'settings.local.discoveryError':'No se pudo detectar el entorno local: {detail}',
  'settings.local.detected':'detectado','settings.local.unavailable':'no disponible','settings.local.configured':'configurado','settings.local.configuring':'Configurando…','settings.local.use':'Usar en Oberth','settings.local.attention':'requiere atención','settings.local.notDetected':'no detectado',
  'settings.local.inferenceProvider':'proveedor de inferencia','settings.local.agentHarness':'agente de desarrollo','settings.local.ready':'Listo para usar.','settings.local.startLMStudio':'Iniciá el servidor local y cargá un modelo desde LM Studio.','settings.local.cliProbeFailed':'Se detectó la CLI, pero falló la comprobación de versión; todavía no puede ejecutarse desde Oberth.','settings.local.authNeeded':'Está instalado, pero requiere autenticación.','settings.local.cliMissing':'La CLI no está instalada o no está disponible en PATH.',
  'settings.local.auth':'autenticación','settings.local.authUnknown':'desconocida','settings.local.authRequired':'requerida','settings.local.authNotVerified':'sin verificar','settings.local.authProbeFailed':'comprobación fallida',
  'settings.providers.title':'Proveedores LLM','settings.providers.active':'activo','settings.providers.inactive':'inactivo','settings.providers.edit':'Editar','settings.providers.verify':'Verificar','settings.providers.verifying':'Verificando…',
  'settings.providers.empty':'No hay proveedores configurados','settings.providers.emptyDetail':'Podés habilitar uno detectado arriba o configurar un servicio manualmente.','settings.providers.editTitle':'Editar proveedor','settings.providers.manual':'Configuración manual',
  'settings.providers.type':'Tipo','settings.providers.name':'Nombre','settings.providers.model':'Modelo','settings.providers.cancel':'Cancelar','settings.providers.saveChanges':'Guardar cambios','settings.providers.save':'Guardar proveedor','settings.providers.keepSecret':'Dejar vacío para conservarla',
  'statusbar.desktop':'aplicación local','statusbar.web':'servidor local','statusbar.vault':'memoria','statusbar.ok':'ok','statusbar.offline':'sin conexión','statusbar.semantic':'búsqueda semántica','statusbar.disabled':'desactivada','statusbar.localFallback':'alternativa local disponible','statusbar.ready':'lista','statusbar.externalVector':'motor vectorial externo','statusbar.builtinVector':'índice vectorial integrado',
}
const catalogs:Record<Locale,Messages>={en,es}
const catalogValues:Record<Locale,Set<string>>={
  en:new Set(Object.values(en)),
  es:new Set(Object.values(es)),
}
const compatLookup:Record<Locale,Map<string,string>>={en:new Map(),es:new Map()}
for(const row of compatTranslations){
  for(const alias of [row.source,row.en,row.es]){
    compatLookup.en.set(alias,row.en)
    compatLookup.es.set(alias,row.es)
  }
}
const reviewedCompatOverrides=[
  {source:'Actualizar',en:'Refresh',es:'Actualizar'},
  {source:'Actualizando\u2026',en:'Refreshing\u2026',es:'Actualizando\u2026'},
  {source:'Opciones de ejecuci\u00f3n',en:'Execution options',es:'Opciones de ejecuci\u00f3n'},
  {source:'Usala solo cuando quieras asignar modelos diferentes a varias etapas.',en:'Use this only when you want to assign different models to multiple stages.',es:'Usala solo cuando quieras asignar modelos diferentes a varias etapas.'},
  {source:'No se pudo completar la acción: timeout',en:'The action could not be completed: timeout',es:'No se pudo completar la acción: timeout'},
  {source:'Ingresar ruta manualmente',en:'Enter path manually',es:'Ingresar ruta manualmente'},
  {source:'Modelo para esta solicitud',en:'Model for this request',es:'Modelo para esta solicitud'},
  {source:'Proveedor',en:'Provider',es:'Proveedor'},
  {source:'Configurar proveedores',en:'Configure providers',es:'Configurar proveedores'},
  {source:'ruta no disponible',en:'path unavailable',es:'ruta no disponible'},
  {source:'tokens seleccionados',en:'selected tokens',es:'tokens seleccionados'},
  {source:'descartadas',en:'discarded',es:'descartadas'},
  {source:'activo',en:'active',es:'activo'},
  {source:'registro de acciones tipadas',en:'typed action log',es:'registro de acciones tipadas'},
  {source:'tokens por revisar',en:'tokens to review',es:'tokens por revisar'},
  {source:'Descartar',en:'Discard',es:'Descartar'},
  {source:'coincidencia',en:'match',es:'coincidencia'},
  {source:'Eliminar',en:'Delete',es:'Eliminar'},
  {source:'Por proveedor',en:'By provider',es:'Por proveedor'},
  {source:'auth required',en:'authentication required',es:'autenticación requerida'},
] as const
for(const row of reviewedCompatOverrides)for(const alias of [row.source,row.en,row.es]){compatLookup.en.set(alias,row.en);compatLookup.es.set(alias,row.es)}
export type MessageKey=keyof Messages
export function savedLocale():Locale{return localStorage.getItem(STORAGE_KEY)==='es'?'es':'en'}
export function translate(locale:Locale,key:MessageKey,values:Record<string,string|number>={}){
  return Object.entries(values).reduce((text,[name,value])=>text.replaceAll(`{${name}}`,String(value)),catalogs[locale][key]??en[key])
}
type ContextValue={locale:Locale;setLocale:(locale:Locale)=>void;t:(key:MessageKey,values?:Record<string,string|number>)=>string}
const defaultValue:ContextValue={locale:'en',setLocale:()=>{},t:(key,values)=>translate('en',key,values)}
const Context=createContext<ContextValue>(defaultValue)
export function I18nProvider({children}:{children:ReactNode}){
  const[locale,setLocaleState]=useState<Locale>(savedLocale)
  const value=useMemo<ContextValue>(()=>({locale,setLocale:next=>{localStorage.setItem(STORAGE_KEY,next);document.documentElement.lang=next;setLocaleState(next)},t:(key,values)=>translate(locale,key,values)}),[locale])
  document.documentElement.lang=locale
  return <Context.Provider value={value}><CompatLocalization locale={locale}>{children}</CompatLocalization></Context.Provider>
}
export function useI18n(){return useContext(Context)}

function CompatLocalization({children,locale}:{children:ReactNode;locale:Locale}){
  useLayoutEffect(()=>{
    const attributes=['aria-label','placeholder','title']
    const protectedTerms=new Set(['Oberth','Ollama','OpenAI','Anthropic','Google','Qdrant','Claude Code','Codex CLI','OpenCode CLI','Antigravity','LM Studio','API key','Base URL','JSON','QA','Git','Vault'])
    const translateValue=(value:string)=>{
      const trimmed=value.trim(),translated=compatLookup[locale].get(trimmed)
      if(/\bv?\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?\b/.test(trimmed))return value
      if(catalogValues[locale].has(trimmed))return value
      if(protectedTerms.has(trimmed))return value
      if(translated!==undefined)return value.replace(trimmed,translated)
      return value
    }
    const localize=(root:Node)=>{
      const element=root.nodeType===Node.TEXT_NODE?root.parentElement:root instanceof Element?root:null
      if(element?.closest('[data-no-translate]'))return
      if(root.nodeType===Node.TEXT_NODE){
        const current=root.nodeValue||'',next=translateValue(current)
        if(next!==current)root.nodeValue=next
        return
      }
      if(root instanceof Element){
        for(const name of attributes){
          const current=root.getAttribute(name)
          if(current!==null){
            const next=translateValue(current)
            if(next!==current)root.setAttribute(name,next)
          }
        }
      }
      for(const child of root.childNodes)localize(child)
    }
    localize(document.body)
    const observer=new MutationObserver(records=>{for(const record of records){if(record.type==='characterData')localize(record.target);else for(const node of record.addedNodes)localize(node)}})
    observer.observe(document.body,{subtree:true,childList:true,characterData:true})
    return()=>observer.disconnect()
  },[locale])
  return children
}
