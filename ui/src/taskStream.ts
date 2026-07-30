export type TaskStreamState = Record<string, string>

export function applyTaskStreamEvent(
  state: TaskStreamState,
  type: 'task.started' | 'task.chunk',
  payload: Record<string, unknown>,
): TaskStreamState {
  const taskID = typeof payload.task_id === 'string' ? payload.task_id : ''
  if (!taskID) return state
  if (type === 'task.started') return { ...state, [taskID]: '' }
  const content = typeof payload.content === 'string' ? payload.content : ''
  if (!content) return state
  return { ...state, [taskID]: (state[taskID] || '') + content }
}
