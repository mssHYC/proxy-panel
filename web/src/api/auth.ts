import request from './request'

export const login = (username: string, password: string) =>
  request.post('/auth/login', { username, password })

export const verify2FA = (temp_token: string, code: string) =>
  request.post('/auth/2fa/verify', { temp_token, code })

export const changePassword = (old_password: string, new_password: string) =>
  request.put('/auth/password', { old_password, new_password })

export const changeUsername = (password: string, new_username: string) =>
  request.put('/auth/username', { password, new_username })

export const get2FAStatus = () =>
  request.get('/auth/2fa/status')

export const setup2FA = (password: string) =>
  request.post('/auth/2fa/setup', { password })

export const enable2FA = (password: string, code: string) =>
  request.post('/auth/2fa/enable', { password, code })

export const disable2FA = (password: string) =>
  request.post('/auth/2fa/disable', { password })
