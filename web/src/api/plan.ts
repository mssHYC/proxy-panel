import request from './request'

export const getNodeGroups = () => request.get('/node-groups')
export const createNodeGroup = (data: any) => request.post('/node-groups', data)
export const updateNodeGroup = (id: number, data: any) => request.put(`/node-groups/${id}`, data)
export const deleteNodeGroup = (id: number) => request.delete(`/node-groups/${id}`)

export const getPlans = () => request.get('/plans')
export const createPlan = (data: any) => request.post('/plans', data)
export const updatePlan = (id: number, data: any) => request.put(`/plans/${id}`, data)
export const deletePlan = (id: number) => request.delete(`/plans/${id}`)
export const assignPlanToUser = (userId: number, data: any) => request.post(`/users/${userId}/plan`, data)
