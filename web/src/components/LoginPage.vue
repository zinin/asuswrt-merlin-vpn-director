<script setup lang="ts">
import { ref } from 'vue'
import api from '../api'

const emit = defineEmits<{ login: [] }>()

const password = ref('')
const error = ref('')
const loading = ref(false)

async function submit() {
  error.value = ''
  loading.value = true
  try {
    const res = await api.login(password.value)
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
      <input v-model="password" type="password" placeholder="Password" autocomplete="current-password" />
      <button class="btn btn-primary" type="submit" :disabled="loading" style="width: 100%; margin-top: 1rem;">
        {{ loading ? 'Logging in...' : 'Login' }}
      </button>
    </form>
  </div>
</template>
