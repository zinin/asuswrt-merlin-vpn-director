<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api'

const status = ref('')
const ip = ref('')
const loading = ref(false)
const actionLoading = ref('')

async function loadStatus() {
  loading.value = true
  try {
    const [statusRes, ipRes] = await Promise.all([
      api.getStatus(),
      api.getIP(),
    ])
    status.value = statusRes.data.output
    ip.value = ipRes.data.ip
  } catch (e: any) {
    status.value = 'Error: ' + (e.response?.data?.error || e.message)
  } finally {
    loading.value = false
  }
}

async function doAction(name: string, fn: () => Promise<any>) {
  actionLoading.value = name
  try {
    await fn()
    await loadStatus()
  } catch (e: any) {
    alert('Error: ' + (e.response?.data?.error || e.message))
  } finally {
    actionLoading.value = ''
  }
}

onMounted(loadStatus)
</script>

<template>
  <div class="actions">
    <button class="btn btn-green" :disabled="!!actionLoading" @click="doAction('apply', api.apply)">
      {{ actionLoading === 'apply' ? '...' : '▶ Apply' }}
    </button>
    <button class="btn btn-yellow" :disabled="!!actionLoading" @click="doAction('restart', api.restart)">
      {{ actionLoading === 'restart' ? '...' : '↻ Restart' }}
    </button>
    <button class="btn btn-red" :disabled="!!actionLoading" @click="doAction('stop', api.stop)">
      {{ actionLoading === 'stop' ? '...' : '■ Stop' }}
    </button>
    <button class="btn btn-blue" :disabled="!!actionLoading" @click="doAction('ipsets', api.updateIPsets)">
      {{ actionLoading === 'ipsets' ? '...' : '⟳ Update IPsets' }}
    </button>
    <button class="btn btn-blue" :disabled="loading" @click="loadStatus">
      {{ loading ? '...' : '⟳ Refresh' }}
    </button>
  </div>

  <div class="grid-2">
    <div class="card">
      <div class="card-title">Status</div>
      <pre style="font-size: 12px; white-space: pre-wrap; line-height: 1.6;">{{ status || 'Loading...' }}</pre>
    </div>
    <div class="card">
      <div class="card-title">External IP</div>
      <div style="font-size: 20px; margin-top: 8px;">{{ ip || '...' }}</div>
    </div>
  </div>
</template>
