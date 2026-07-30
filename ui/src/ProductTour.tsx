import {useState} from 'react'
import {ArrowLeft,ArrowRight,BookOpen,Check,HelpCircle,Settings,SquareTerminal,X} from 'lucide-react'
import Modal from './Modal'
import {MessageKey,useI18n} from './i18n'
import './product-tour.css'

export const TOUR_STORAGE_KEY='oberth.product-tour.v1'
export type TourTarget='dash'|'settings'|'sess'|'vault'|'costs'

const steps:{target:TourTarget;Icon:typeof BookOpen;title:MessageKey;body:MessageKey;hint:MessageKey}[]=[
  {target:'dash',Icon:SquareTerminal,title:'tour.work.title',body:'tour.work.body',hint:'tour.work.hint'},
  {target:'settings',Icon:Settings,title:'tour.model.title',body:'tour.model.body',hint:'tour.model.hint'},
  {target:'sess',Icon:SquareTerminal,title:'tour.task.title',body:'tour.task.body',hint:'tour.task.hint'},
  {target:'sess',Icon:Check,title:'tour.review.title',body:'tour.review.body',hint:'tour.review.hint'},
  {target:'vault',Icon:BookOpen,title:'tour.memory.title',body:'tour.memory.body',hint:'tour.memory.hint'},
]

export default function ProductTour({onNavigate}:{onNavigate:(target:TourTarget)=>void}){
  const{t}=useI18n()
  const[firstVisit]=useState(()=>localStorage.getItem(TOUR_STORAGE_KEY)!=='completed')
  const[open,setOpen]=useState(firstVisit)
  const[index,setIndex]=useState(0)
  const step=steps[index]
  const show=()=>{setIndex(0);setOpen(true);onNavigate(steps[0].target)}
  const close=(completed=false)=>{if(completed)localStorage.setItem(TOUR_STORAGE_KEY,'completed');setOpen(false)}
  const move=(next:number)=>{setIndex(next);onNavigate(steps[next].target)}

  return <>
    <button className="tour-launcher" aria-label={t('tour.open')} title={t('tour.title')} onClick={show}><HelpCircle size={15}/></button>
    <Modal open={open} label={t('tour.title')} onClose={()=>close()} backdropClassName="tour-backdrop" dialogClassName="tour-dialog">
      <section className="tour-card">
        <header><span className="tour-kicker">{t('tour.title')}</span><button aria-label={t('tour.close')} onClick={()=>close()}><X size={15}/></button></header>
        <div className="tour-progress"><span>{t('tour.step',{current:index+1,total:steps.length})}</span><progress aria-label={t('tour.step',{current:index+1,total:steps.length})} value={index+1} max={steps.length}/></div>
        <div className="tour-content"><span className="tour-icon"><step.Icon size={18}/></span><h2>{t(step.title)}</h2><p>{t(step.body)}</p><aside>{t(step.hint)}</aside></div>
        <footer><button className="tour-skip" onClick={()=>close(true)}>{t('tour.skip')}</button><div>{index>0&&<button onClick={()=>move(index-1)}><ArrowLeft size={13}/>{t('tour.back')}</button>}{index<steps.length-1?<button className="primary" onClick={()=>move(index+1)}>{t('tour.next')}<ArrowRight size={13}/></button>:<button className="primary" onClick={()=>close(true)}><Check size={13}/>{t('tour.finish')}</button>}</div></footer>
      </section>
    </Modal>
  </>
}
