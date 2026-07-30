import {useEffect,useRef,type ReactNode} from 'react'
import {createPortal} from 'react-dom'

type ModalProps={
  open:boolean
  label:string
  onClose:()=>void
  backdropClassName?:string
  dialogClassName?:string
  children:ReactNode
}

export default function Modal({open,label,onClose,backdropClassName='modal-backdrop',dialogClassName='modal-dialog',children}:ModalProps){
  const dialogRef=useRef<HTMLDivElement>(null)
  const previousFocus=useRef<HTMLElement|null>(null)
  const onCloseRef=useRef(onClose)
  onCloseRef.current=onClose

  useEffect(()=>{
    if(!open)return
    previousFocus.current=document.activeElement instanceof HTMLElement?document.activeElement:null
    const previousOverflow=document.body.style.overflow
    document.body.style.overflow='hidden'
    const dialog=dialogRef.current
    dialog?.querySelector<HTMLElement>('button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])')?.focus()
    const onKeyDown=(event:KeyboardEvent)=>{
      if(event.key==='Escape'){event.preventDefault();onCloseRef.current();return}
      if(event.key!=='Tab'||!dialog)return
      const controls=Array.from(dialog.querySelectorAll<HTMLElement>('button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])'))
      if(!controls.length)return
      const first=controls[0],last=controls[controls.length-1]
      if(event.shiftKey&&document.activeElement===first){event.preventDefault();last.focus()}
      else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first.focus()}
    }
    document.addEventListener('keydown',onKeyDown)
    return()=>{
      document.removeEventListener('keydown',onKeyDown)
      document.body.style.overflow=previousOverflow
      previousFocus.current?.focus()
    }
  },[open])

  if(!open)return null
  return createPortal(
    <div className={backdropClassName} onMouseDown={event=>{if(event.target===event.currentTarget)onCloseRef.current()}}>
      <div ref={dialogRef} className={dialogClassName} role="dialog" aria-modal="true" aria-label={label}>{children}</div>
    </div>,
    document.body,
  )
}
