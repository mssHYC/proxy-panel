import request from './request'

export const getServerTraffic = () => request.get('/traffic/server')
export const setServerLimit = (limitGB: number) => request.post('/traffic/server/limit', { limit_gb: limitGB })
export const getTrafficHistory = (days: number = 30) => request.get('/traffic/history', { params: { days } })
export const getTrafficByNode = (days: number = 30) => request.get('/traffic/by-node', { params: { days } })
