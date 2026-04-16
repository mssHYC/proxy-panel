import request from './request'

export const login = (username: string, password: string) =>
  request.post('/auth/login', { username, password })
