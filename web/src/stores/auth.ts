import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem('token') || '')

  function setToken(t: string) {
    token.value = t
    localStorage.setItem('token', t)
  }

  function logout() {
    token.value = ''
    localStorage.removeItem('token')
  }

  const isLoggedIn = () => !!token.value

  return { token, setToken, logout, isLoggedIn }
})
