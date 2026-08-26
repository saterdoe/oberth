import {fireEvent,render,screen} from '@testing-library/react'
import {beforeEach,describe,expect,it,vi} from 'vitest'
import ProductTour,{TOUR_STORAGE_KEY} from './ProductTour'

describe('product tour',()=>{
  beforeEach(()=>localStorage.clear())

  it('guides first-time users through the product and persists completion',()=>{
    const navigate=vi.fn()
    render(<ProductTour onNavigate={navigate}/>)

    expect(screen.getByRole('dialog',{name:'Product tour'})).toBeInTheDocument()
    expect(screen.getByText('One place to follow the work')).toBeInTheDocument()
    expect(screen.getByText('Step 1 of 5')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button',{name:'Next'}))
    expect(screen.getByText('First, connect a model')).toBeInTheDocument()
    expect(navigate).toHaveBeenCalledWith('settings')

    for(let step=0;step<3;step++)fireEvent.click(screen.getByRole('button',{name:'Next'}))
    fireEvent.click(screen.getByRole('button',{name:'Finish'}))

    expect(screen.queryByRole('dialog',{name:'Product tour'})).not.toBeInTheDocument()
    expect(localStorage.getItem(TOUR_STORAGE_KEY)).toBe('completed')
  })

  it('can be replayed after completion',()=>{
    localStorage.setItem(TOUR_STORAGE_KEY,'completed')
    render(<ProductTour onNavigate={()=>undefined}/>)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button',{name:'Open guided tour'}))
    expect(screen.getByRole('dialog',{name:'Product tour'})).toBeInTheDocument()
  })

  it('closes when clicking outside the tour',()=>{
    render(<ProductTour onNavigate={()=>undefined}/>)
    const dialog=screen.getByRole('dialog',{name:'Product tour'})
    fireEvent.mouseDown(dialog.parentElement!)
    expect(screen.queryByRole('dialog',{name:'Product tour'})).not.toBeInTheDocument()
  })
})
