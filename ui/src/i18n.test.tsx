import {fireEvent,render,screen} from '@testing-library/react'
import {beforeEach,describe,expect,it} from 'vitest'
import {catalogs,I18nProvider,translate,useI18n} from './i18n'
import {formatters} from './format'

function Probe(){const{locale,setLocale,t}=useI18n();return <><span>{locale}</span><strong>{t('dashboard.newTask')}</strong><span>{t('nav.costs')}</span><button aria-label={t('taskWorkspace.refresh')}>{t('taskWorkspace.refresh')}</button><button onClick={()=>setLocale(locale==='en'?'es':'en')}>switch</button></>}
const immutable=['Oberth v0.1.0-alpha.9','Ollama','tokens','projects/4a280ac68fd9/sessions/721fb570','System','Proveedor','Revisá los cambios y decidí','Cambios: árbol, sesión, configuración.']
describe('typed localization',()=>{
  beforeEach(()=>localStorage.clear())
  it('defaults to English, switches product copy and persists Spanish',()=>{
    const view=render(<I18nProvider><Probe/>{immutable.map(text=><p key={text}>{text}</p>)}</I18nProvider>)
    expect(screen.getByText('New task')).toBeInTheDocument()
    expect(screen.getByRole('button',{name:'Refresh'})).toHaveTextContent('Refresh')
    fireEvent.click(screen.getByRole('button',{name:'switch'}))
    expect(screen.getByText('Nueva tarea')).toBeInTheDocument()
    expect(screen.getByText('Costos')).toBeInTheDocument()
    expect(screen.getByRole('button',{name:'Actualizar'})).toHaveTextContent('Actualizar')
    for(const text of immutable)expect(screen.getByText(text)).toBeInTheDocument()
    expect(localStorage.getItem('oberth.locale')).toBe('es')
    expect(document.documentElement.lang).toBe('es')
    view.unmount();render(<I18nProvider><Probe/></I18nProvider>)
    expect(screen.getByText('Nueva tarea')).toBeInTheDocument()
  })
  it('has matching keys and interpolation parameters in every catalog',()=>{
    expect(Object.keys(catalogs.es).sort()).toEqual(Object.keys(catalogs.en).sort())
    for(const key of Object.keys(catalogs.en) as Array<keyof typeof catalogs.en>){
      expect(catalogs.es[key].trim(),key).not.toBe('')
      const params=(value:string)=>[...value.matchAll(/\{(\w+)\}/g)].map(match=>match[1]).sort()
      expect(params(catalogs.es[key]),key).toEqual(params(catalogs.en[key]))
    }
  })
  it('interpolates once without reinterpreting user text',()=>{
    expect(translate('en','settings.providers.testError',{provider:'{detail}',detail:'tokens sesión'})).toBe('Could not verify {detail}: tokens sesión')
    expect(translate('es','dashboard.total',{count:12345})).toBe('12.345 en total')
  })
  it('formats singular, plural and zero with Intl rules',()=>{
    expect(translate('en','session.filesReady',{count:1})).toBe('1 file ready to review')
    expect(translate('en','session.filesReady',{count:2})).toBe('2 files ready to review')
    expect(translate('es','common.files',{count:0})).toBe('0 archivos')
    expect(translate('es','common.chunks',{count:1})).toBe('1 fragmento')
  })
  it('formats costs and dates by locale, including invalid and future timestamps',()=>{
    const en=formatters('en'),es=formatters('es'),now=Date.parse('2026-08-28T12:00:00Z')
    expect(en.money(12.5,2)).toBe('$12.50')
    expect(es.money(12.5,2)).toContain('12,50')
    expect(en.relative('2026-08-28T11:58:00Z',now)).toBe('2 minutes ago')
    expect(es.relative('2026-08-28T12:02:00Z',now)).toBe('dentro de 2 minutos')
    expect(es.date('invalid')).toBe('—')
    expect(en.relative('invalid',now)).toBe('—')
  })
})
