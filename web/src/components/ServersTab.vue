<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api'
import type { Server } from '../types'

const servers = ref<Server[]>([])
const loading = ref(false)
const importLoading = ref(false)
const selectLoading = ref('')
const error = ref('')

async function loadServers() {
  loading.value = true
  error.value = ''
  try {
    const resp = await api.getServers()
    servers.value = resp.data.servers ?? []
  } catch (e: any) {
    error.value = e.response?.data?.error || e.message
  } finally {
    loading.value = false
  }
}

async function selectServer(name: string) {
  selectLoading.value = name
  try {
    await api.selectServer(name)
    alert('Server selected: ' + name)
    await loadServers()
  } catch (e: any) {
    alert('Error: ' + (e.response?.data?.error || e.message))
  } finally {
    selectLoading.value = ''
  }
}

async function importServers() {
  importLoading.value = true
  try {
    await api.importServers()
    await loadServers()
  } catch (e: any) {
    alert('Error: ' + (e.response?.data?.error || e.message))
  } finally {
    importLoading.value = false
  }
}

onMounted(loadServers)
</script>

<template>
  <div class="card">
    <div class="card-title">Servers</div>

    <div class="actions">
      <button class="btn btn-blue" :disabled="loading" @click="loadServers">
        {{ loading ? '...' : '⟳ Refresh' }}
      </button>
      <button class="btn btn-primary" :disabled="importLoading" @click="importServers">
        {{ importLoading ? '...' : '⬇ Import Subscription' }}
      </button>
    </div>

    <p v-if="error" class="error-msg">{{ error }}</p>

    <table v-if="servers.length > 0">
      <thead>
        <tr>
          <th>#</th>
          <th>Name</th>
          <th>Address</th>
          <th>Port</th>
          <th>Action</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(server, idx) in servers" :key="idx">
          <td>{{ idx + 1 }}</td>
          <td>{{ server.name }}</td>
          <td>{{ server.address }}</td>
          <td>{{ server.port }}</td>
          <td>
            <button
              class="btn btn-green"
              :disabled="!!selectLoading"
              @click="selectServer(server.name)"
            >
              {{ selectLoading === server.name ? '...' : 'Select' }}
            </button>
          </td>
        </tr>
      </tbody>
    </table>

    <p v-else-if="!loading" style="color: #999; font-size: 0.875rem;">
      No servers found. Import a subscription to get started.
    </p>
  </div>
</template>
