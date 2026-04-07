<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import api from '../api'

const logs = ref('')
const loading = ref(false)
const lines = ref(50)
const error = ref('')

async function loadLogs() {
  loading.value = true
  error.value = ''
  try {
    const resp = await api.getLogs(lines.value)
    logs.value = resp.data.output ?? ''
  } catch (e: any) {
    error.value = e.response?.data?.error || e.message
  } finally {
    loading.value = false
  }
}

watch(lines, () => {
  loadLogs()
})

onMounted(loadLogs)
</script>

<template>
  <div class="card">
    <div class="card-title">Logs</div>

    <div style="display: flex; gap: 0.5rem; align-items: center; margin-bottom: 0.75rem; flex-wrap: wrap;">
      <div class="form-group" style="width: 120px; margin-bottom: 0;">
        <label>Lines</label>
        <input v-model.number="lines" type="number" min="10" max="500" />
      </div>
      <button class="btn btn-blue" :disabled="loading" @click="loadLogs" style="align-self: flex-end;">
        {{ loading ? '...' : '⟳ Refresh' }}
      </button>
    </div>

    <p v-if="error" class="error-msg">{{ error }}</p>

    <pre style="font-size: 11px; white-space: pre-wrap; line-height: 1.5; max-height: 500px; overflow-y: auto; background: #1a1a2e; padding: 0.75rem; border-radius: 4px; border: 1px solid #333;">{{ logs || (loading ? 'Loading...' : 'No logs available.') }}</pre>
  </div>
</template>
