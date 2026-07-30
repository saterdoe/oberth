import {fireEvent,render,screen} from '@testing-library/react'
import {describe,expect,it,vi} from 'vitest'
import Modal from './Modal'

describe('Modal',()=>{
  it('closes from the backdrop and Escape, but not from the dialog content',()=>{
    const onClose=vi.fn()
    render(<Modal open label="Prueba" onClose={onClose}><button>Acción</button></Modal>)
    const dialog=screen.getByRole('dialog',{name:'Prueba'})

    fireEvent.mouseDown(dialog)
    expect(onClose).not.toHaveBeenCalled()

    fireEvent.mouseDown(dialog.parentElement!)
    expect(onClose).toHaveBeenCalledTimes(1)

    fireEvent.keyDown(document,{key:'Escape'})
    expect(onClose).toHaveBeenCalledTimes(2)
  })

  it('moves focus inside and restores it after closing',()=>{
    const opener=document.createElement('button')
    document.body.appendChild(opener)
    opener.focus()
    const {rerender}=render(<Modal open label="Prueba" onClose={()=>undefined}><button>Acción</button></Modal>)
    expect(screen.getByRole('button',{name:'Acción'})).toHaveFocus()

    rerender(<Modal open={false} label="Prueba" onClose={()=>undefined}><button>Acción</button></Modal>)
    expect(opener).toHaveFocus()
    opener.remove()
  })

  it('wraps keyboard focus between the first and last controls',()=>{
    render(<Modal open label="Keyboard dialog" onClose={()=>{}}>
      <button>First</button>
      <button>Last</button>
    </Modal>)
    const first=screen.getByRole('button',{name:'First'})
    const last=screen.getByRole('button',{name:'Last'})

    last.focus()
    fireEvent.keyDown(document,{key:'Tab'})
    expect(first).toHaveFocus()
    first.focus()
    fireEvent.keyDown(document,{key:'Tab',shiftKey:true})
    expect(last).toHaveFocus()
  })

  it('ignores tab trapping when the dialog has no controls',()=>{
    render(<Modal open label="Empty dialog" onClose={()=>{}}><span>Empty</span></Modal>)
    expect(()=>fireEvent.keyDown(document,{key:'Tab'})).not.toThrow()
  })
})
