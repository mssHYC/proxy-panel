import request from './request'

export const getAuditLogs = (params: Record<string, any>) =>
  request.get('/audit-logs', { params })

export const exportAuditLogs = (params: Record<string, any>) =>
  request.get('/audit-logs/export', { params, responseType: 'blob', silent: true } as any)
