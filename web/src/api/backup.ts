import request from './request'

export const exportBackup = () =>
  request.get('/backup/export', { responseType: 'blob' })

export const importBackup = (file: File) => {
  const form = new FormData()
  form.append('file', file)
  return request.post('/backup/import', form, { headers: { 'Content-Type': 'multipart/form-data' } })
}
