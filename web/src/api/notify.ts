import request from './request'

export const testNotify = (channel?: string) => request.post('/notify/test', { channel })
