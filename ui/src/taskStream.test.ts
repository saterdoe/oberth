import { describe, expect, it } from 'vitest'
import { applyTaskStreamEvent, type TaskStreamState } from './taskStream'

describe('task stream reducer', () => {
  it('resets on start and appends ordered chunks for the selected task', () => {
    let state: TaskStreamState = { other: 'keep', task: 'old' }
    state = applyTaskStreamEvent(state, 'task.started', { task_id: 'task' })
    state = applyTaskStreamEvent(state, 'task.chunk', { task_id: 'task', content: 'hello ' })
    state = applyTaskStreamEvent(state, 'task.chunk', { task_id: 'task', content: 'world' })
    expect(state).toEqual({ other: 'keep', task: 'hello world' })
  })

  it('ignores malformed events', () => {
    const state = { task: 'safe' }
    expect(applyTaskStreamEvent(state, 'task.chunk', { content: 'bad' })).toBe(state)
  })
})
