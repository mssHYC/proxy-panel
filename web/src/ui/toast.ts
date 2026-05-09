import { toast as sonner } from 'vue-sonner'

type Tone = 'success' | 'error' | 'warning' | 'info'

function show(tone: Tone, msg: string, opts: Record<string, any> = {}) {
  const fn = (sonner as any)[tone] ?? sonner
  return fn(msg, opts)
}

export const toast = Object.assign(
  (msg: string, opts?: Record<string, any>) => show('info', msg, opts || {}),
  {
    success: (msg: string, opts?: Record<string, any>) => show('success', msg, opts || {}),
    error:   (msg: string, opts?: Record<string, any>) => show('error',   msg, opts || {}),
    warn:    (msg: string, opts?: Record<string, any>) => show('warning', msg, opts || {}),
    info:    (msg: string, opts?: Record<string, any>) => show('info',    msg, opts || {}),
  },
)
