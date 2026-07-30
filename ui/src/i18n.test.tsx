import {fireEvent,render,screen} from '@testing-library/react'
import {beforeEach,describe,expect,it} from 'vitest'
import {I18nProvider,translate,useI18n} from './i18n'

function Probe(){const{locale,setLocale,t}=useI18n();return <><span>{locale}</span><strong>{t('dashboard.newTask')}</strong><span>{t('nav.costs')}</span><code>Oberth v0.1.0-alpha.1</code><button onClick={()=>setLocale('es')}>switch</button></>}
function LegacyProbe(){const{setLocale}=useI18n();return <><p>Revisá los cambios y decidí</p><p>No se pudo completar la acción: timeout</p><span>Ollama</span><button onClick={()=>setLocale('es')}>legacy-switch</button></>}
describe('i18n',()=>{
  beforeEach(()=>localStorage.clear())
  it('defaults to English and persists Spanish',()=>{
    const view=render(<I18nProvider><Probe/></I18nProvider>)
    expect(screen.getByText('New task')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button',{name:'switch'}))
    expect(screen.getByText('Nueva tarea')).toBeInTheDocument()
    expect(screen.getByText('Costos')).toBeInTheDocument()
    expect(screen.queryByText('Costoos')).not.toBeInTheDocument()
    expect(screen.getByText('Oberth v0.1.0-alpha.1')).toBeInTheDocument()
    expect(screen.queryByText(/alfa/)).not.toBeInTheDocument()
    expect(localStorage.getItem('oberth.locale')).toBe('es')
    view.unmount()
    render(<I18nProvider><Probe/></I18nProvider>)
    expect(screen.getByText('Nueva tarea')).toBeInTheDocument()
  })
  it('interpolates catalog values',()=>{expect(translate('en','dashboard.total',{count:3})).toBe('3 total');expect(translate('es','dashboard.total',{count:3})).toBe('3 en total')})
  it('localizes legacy and dynamic copy without translating product names',()=>{
    render(<I18nProvider><LegacyProbe/></I18nProvider>)
    expect(screen.getByText('Review the changes and decide')).toBeInTheDocument()
    expect(screen.getByText('The action could not be completed: timeout')).toBeInTheDocument()
    expect(screen.getByText('Ollama')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button',{name:'legacy-switch'}))
    expect(screen.getByText('Revisá los cambios y decidí')).toBeInTheDocument()
  })
})
