<script setup lang="ts">
import { ref } from 'vue'
import api from '../api'

const emit = defineEmits<{ login: [] }>()

const username = ref('admin')
const password = ref('')
const error = ref('')
const loading = ref(false)

async function submit() {
  error.value = ''
  loading.value = true
  try {
    const res = await api.login(username.value, password.value)
    localStorage.setItem('token', res.data.token)
    emit('login')
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Connection failed'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <form class="login-box" @submit.prevent="submit">
      <h1>VPN Director</h1>
      <div class="error-msg" v-if="error">{{ error }}</div>
      <input v-model="username" type="text" placeholder="Username" autocomplete="username" />
      <input v-model="password" type="password" placeholder="Password" autocomplete="current-password" style="margin-top: 0.5rem;" />
      <button class="btn btn-primary" type="submit" :disabled="loading" style="width: 100%; margin-top: 1rem;">
        {{ loading ? 'Logging in...' : 'Login' }}
      </button>
    </form>
  </div>
</template>
