import request from './request'

export const getNodes = () => request.get('/nodes')
export const createNode = (data: any) => request.post('/nodes', data)
export const updateNode = (id: number, data: any) => request.put(`/nodes/${id}`, data)
export const deleteNode = (id: number) => request.delete(`/nodes/${id}`)
