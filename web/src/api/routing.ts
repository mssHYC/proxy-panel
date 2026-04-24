import request from './request'
import type { RoutingConfig, Category, Group, CustomRule } from '@/views/settings/routing/types'

export const getRoutingConfig = () => request.get<RoutingConfig>('/routing/config')

export const createCategory = (body: Partial<Category>) => request.post('/routing/categories', body)
export const updateCategory = (id: number, body: Partial<Category>) => request.put(`/routing/categories/${id}`, body)
export const deleteCategory = (id: number) => request.delete(`/routing/categories/${id}`)

export const createGroup = (body: Partial<Group>) => request.post('/routing/groups', body)
export const updateGroup = (id: number, body: Partial<Group>) => request.put(`/routing/groups/${id}`, body)
export const deleteGroup = (id: number) => request.delete(`/routing/groups/${id}`)

export const createCustomRule = (body: Partial<CustomRule>) => request.post('/routing/custom-rules', body)
export const updateCustomRule = (id: number, body: Partial<CustomRule>) => request.put(`/routing/custom-rules/${id}`, body)
export const deleteCustomRule = (id: number) => request.delete(`/routing/custom-rules/${id}`)

export const applyPreset = (code: string) => request.post('/routing/apply-preset', { code })
export const importLegacy = (text: string, mode: 'prepend' | 'override') => request.post('/routing/import-legacy', { text, mode })
