import axe from 'axe-core'
import {fireEvent,render,screen} from '@testing-library/react'
import {afterEach,expect,it,vi} from 'vitest'
import App from './App'

const response=(data:unknown)=>Promise.resolve(new Response(JSON.stringify({data}),{headers:{'Content-Type':'application/json'}}))

afterEach(()=>vi.unstubAllGlobals())

it('has no serious WCAG violations across critical workspaces and dialogs',async()=>{
  document.documentElement.lang='en'
  document.title='Oberth — Control room'
  vi.stubGlobal('fetch',vi.fn((input:RequestInfo|URL)=>{
    const url=String(input)
    if(url.includes('/tasks?'))return response({tasks:[]})
    if(url.includes('/sessions?'))return response({sessions:[]})
    if(url.endsWith('/status'))return response({server:{state:'healthy',version:'test'}})
    return response([])
  }))
  render(<App/>)
  await screen.findByRole('button',{name:'Dashboard'})

  const audit=async(label:string)=>{
    const result=await axe.run(document,{runOnly:{type:'tag',values:['wcag2a','wcag2aa','wcag21aa','wcag22aa']}})
    const blocking=result.violations.filter(item=>item.impact==='serious'||item.impact==='critical')
    expect(blocking.map(item=>`${label}: ${item.id} (${item.nodes.length})`)).toEqual([])
  }

  await audit('dashboard')
  fireEvent.click(screen.getByRole('button',{name:'Session'}))
  await audit('session')
  fireEvent.click(screen.getByRole('button',{name:'Open product tour'}))
  await screen.findByRole('dialog',{name:'Product tour'})
  await audit('product tour dialog')
})
