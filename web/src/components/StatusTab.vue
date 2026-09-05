<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api'

const status = ref('')
const ip = ref('')
const ipError = ref('')
const loading = ref(false)
const actionLoading = ref('')

function errorText(e: any): string {
  return e?.response?.data?.error || e?.message || 'unknown error'
}

async function loadStatus() {
  loading.value = true
  ipError.value = ''
  // Status and external IP load independently: a failing curl ifconfig.me
  // must not hide the VPN Director status.
  const [statusRes, ipRes] = await Promise.allSettled([api.getStatus(), api.getIP()])
  if (statusRes.status === 'fulfilled') {
    status.value = statusRes.value.data.output
  } else {
    status.value = 'Error: ' + errorText(statusRes.reason)
  }
  if (ipRes.status === 'fulfilled') {
    ip.value = ipRes.value.data.ip
  } else {
    ip.value = ''
    ipError.value = errorText(ipRes.reason)
  }
  loading.value = false
}

async function doAction(name: string, fn: () => Promise<any>) {
  actionLoading.value = name
  try {
    await fn()
    await loadStatus()
  } catch (e: any) {
    alert('Error: ' + errorText(e))
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
      <div v-if="ip" style="font-size: 20px; margin-top: 8px;">{{ ip }}</div>
      <div v-else-if="ipError" style="margin-top: 8px; color: #ff6b6b; font-size: 0.875rem;">
        unavailable: {{ ipError }}
      </div>
      <div v-else style="font-size: 20px; margin-top: 8px;">...</div>
    </div>
  </div>
</template>
