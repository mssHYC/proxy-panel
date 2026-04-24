import request from './request'

export interface SubscriptionToken {
  id: number
  user_id: number
  name: string
  token: string
  enabled: boolean
  expires_at: string | null
  ip_bind_enabled: boolean
  bound_ip: string
  last_ip: string
  last_ua: string
  last_used_at: string | null
  use_count: number
  created_at: string
}

export interface CreateTokenPayload {
  name: string
  expires_at?: string | null
  ip_bind_enabled?: boolean
}

export interface UpdateTokenPayload {
  name?: string
  enabled?: boolean
  expires_at?: string | null
  expires_at_null?: boolean
  ip_bind_enabled?: boolean
  reset_bind?: boolean
}

export const listSubTokens = (userId: number) =>
  request.get<SubscriptionToken[]>(`/users/${userId}/sub-tokens`)

export const createSubToken = (userId: number, payload: CreateTokenPayload) =>
  request.post<SubscriptionToken>(`/users/${userId}/sub-tokens`, payload)

export const updateSubToken = (id: number, payload: UpdateTokenPayload) =>
  request.patch<SubscriptionToken>(`/sub-tokens/${id}`, payload)

export const rotateSubToken = (id: number) =>
  request.post<SubscriptionToken>(`/sub-tokens/${id}/rotate`)

export const deleteSubToken = (id: number) => request.delete(`/sub-tokens/${id}`)
