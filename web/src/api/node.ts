import request from './request'

export const getNodes = () => request.get('/nodes')
export const createNode = (data: any) => request.post('/nodes', data)
export const updateNode = (id: number, data: any) => request.put(`/nodes/${id}`, data, { timeout: 120000 })
export const deleteNode = (id: number) => request.delete(`/nodes/${id}`)
export const generateRealityKeypair = () => request.post('/nodes/generate-reality-keypair')

export const checkRealityTarget = (data: any) => request.post('/nodes/check-reality-target', data)

export interface RealityMirror {
  host: string
  country: string
}

export interface RealityCandidate {
  host: string
  status: string
  message: string
  latency_ms?: number
  cdn_provider: string
  cdn_evidence?: string
}

// The component displays inline errors and aborts these reads when the dialog closes.
const candidateRequestOptions = (signal: AbortSignal) => ({ signal, timeout: 30000, silent: true })

export const getRealityCatalog = (signal: AbortSignal) =>
  request.get<{ mirrors: RealityMirror[] }>('/nodes/reality-candidate-catalog', candidateRequestOptions(signal))

export const checkRealityCandidates = (hosts: string[], signal: AbortSignal) =>
  request.post<{ results: RealityCandidate[] }>('/nodes/check-reality-candidates', { hosts }, candidateRequestOptions(signal))
