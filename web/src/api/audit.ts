import request from './request'

export const getAuditLogs = (params: Record<string, any>) =>
  request.get('/audit-logs', { params })
