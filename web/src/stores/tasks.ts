import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api, call, type Schemas } from '@/api/client'

export type Task = Schemas['Task']

/**
 * Server tasks (reindex, backup, import, …) started from this ds-ui. Updates
 * arrive as server-sent events; the list is loaded once and then kept live.
 */
export const useTasksStore = defineStore('tasks', () => {
  const tasks = ref<Task[]>([])
  let source: EventSource | undefined

  const running = computed(() => tasks.value.filter((t) => t.status === 'running'))

  function upsert(t: Task) {
    const i = tasks.value.findIndex((x) => x.id === t.id)
    if (i >= 0) tasks.value.splice(i, 1, t)
    else tasks.value.unshift(t)
  }

  async function start() {
    try {
      tasks.value = await call(api.GET('/api/v1/tasks'))
    } catch {
      // Not fatal: live events still fill the list.
    }
    if (source) return
    source = new EventSource('/api/v1/events')
    source.addEventListener('task', (e) => upsert(JSON.parse((e as MessageEvent).data) as Task))
  }

  function stop() {
    source?.close()
    source = undefined
  }

  /** Resolves when the task leaves the running state. */
  function whenDone(id: string): Promise<Task> {
    return new Promise((resolve) => {
      const check = () => {
        const t = tasks.value.find((x) => x.id === id)
        if (t && t.status !== 'running') resolve(t)
        else setTimeout(check, 300)
      }
      check()
    })
  }

  return { tasks, running, start, stop, upsert, whenDone }
})
