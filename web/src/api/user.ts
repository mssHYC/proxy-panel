import request from './request'

export const getUsers = () => request.get('/users')
export const getUser = (id: number) => request.get(`/users/${id}`)
export const createUser = (data: any) => request.post('/users', data)
export const updateUser = (id: number, data: any) => request.put(`/users/${id}`, data)
export const deleteUser = (id: number) => request.delete(`/users/${id}`)
export const resetTraffic = (id: number) => request.post(`/users/${id}/reset-traffic`)
