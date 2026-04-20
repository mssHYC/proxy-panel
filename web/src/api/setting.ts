import request from './request'

export const getSettings = () => request.get('/settings')
export const updateSettings = (data: Record<string, string>) => request.put('/settings', data)
export const probeFirewall = (backend: string) => request.post('/firewall/probe', { backend })
export const applyFirewall = () => request.post('/firewall/apply')
