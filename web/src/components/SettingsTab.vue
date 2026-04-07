<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api'
import type { VersionResponse } from '../types'

const versionInfo = ref<VersionResponse | null>(null)
const config = ref('')
const showConfig = ref(false)
const loading = ref(false)
const updateLoading = ref(false)
const error = ref('')

async function loadVersion() {
  try {
    const resp = await api.getVersion()
    versionInfo.value = resp.data
  } catch (e: any) {
    error.value = e.response?.data?.error || e.message
  }
}

async function loadConfig() {
  loading.value = true
  try {
    const resp = await api.getConfig()
    config.value = JSON.stringify(resp.data, null, 2)
  } catch (e: any) {
    error.value = e.response?.data?.error || e.message
  } finally {
    loading.value = false
  }
}

async function toggleConfig() {
  showConfig.value = !showConfig.value
  if (showConfig.value && !config.value) {
    await loadConfig()
  }
}

async function doUpdate() {
  if (!confirm('Update VPN Director to the latest version?')) return
  updateLoading.value = true
  try {
    await api.update()
    alert('Update completed. Please reload the page.')
  } catch (e: any) {
    alert('Error: ' + (e.response?.data?.error || e.message))
  } finally {
    updateLoading.value = false
  }
}

onMounted(loadVersion)
</script>

<template>
  <div class="card">
    <div class="card-title">Version</div>
    <p v-if="error" class="error-msg">{{ error }}</p>
    <div v-if="versionInfo">
      <div class="kv">
        <span class="kv-label">Version</span>
        <span>{{ versionInfo.version }}</span>
      </div>
      <div class="kv">
        <span class="kv-label">Commit</span>
        <span style="font-family: monospace; font-size: 0.85rem;">{{ versionInfo.commit }}</span>
      </div>
    </div>
    <p v-else style="color: #999; font-size: 0.875rem;">Loading...</p>
  </div>

  <div class="actions">
    <button class="btn btn-primary" :disabled="updateLoading" @click="doUpdate">
      {{ updateLoading ? 'Updating...' : '⬆ Update' }}
    </button>
  </div>

  <div class="card">
    <div class="card-title">Configuration</div>
    <div class="actions">
      <button class="btn" @click="toggleConfig">
        {{ showConfig ? 'Hide Config' : 'Show Config' }}
      </button>
      <button v-if="showConfig" class="btn btn-blue" :disabled="loading" @click="loadConfig">
        {{ loading ? '...' : '⟳ Reload' }}
      </button>
    </div>
    <pre
      v-if="showConfig"
      style="font-size: 11px; white-space: pre-wrap; line-height: 1.5; max-height: 500px; overflow-y: auto; background: #1a1a2e; padding: 0.75rem; border-radius: 4px; border: 1px solid #333;"
    >{{ config || 'Loading...' }}</pre>
  </div>
</template>
