<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api'
import type { ClientInfo } from '../types'

const clients = ref<ClientInfo[]>([])
const loading = ref(false)
const actionLoading = ref('')
const error = ref('')

const newIp = ref('')
const newRoute = ref('xray')
const addLoading = ref(false)

const routeOptions = [
  'xray',
  'wgc1', 'wgc2', 'wgc3', 'wgc4', 'wgc5',
  'ovpnc1', 'ovpnc2', 'ovpnc3', 'ovpnc4', 'ovpnc5',
]

async function loadClients() {
  loading.value = true
  error.value = ''
  try {
    const resp = await api.getClients()
    clients.value = resp.data.clients ?? []
  } catch (e: any) {
    error.value = e.response?.data?.error || e.message
  } finally {
    loading.value = false
  }
}

async function addClient() {
  if (!newIp.value.trim()) return
  addLoading.value = true
  try {
    await api.addClient(newIp.value.trim(), newRoute.value)
    newIp.value = ''
    newRoute.value = 'xray'
    await loadClients()
  } catch (e: any) {
    alert('Error: ' + (e.response?.data?.error || e.message))
  } finally {
    addLoading.value = false
  }
}

async function pauseClient(ip: string) {
  actionLoading.value = 'pause:' + ip
  try {
    await api.pauseClient(ip)
    await loadClients()
  } catch (e: any) {
    alert('Error: ' + (e.response?.data?.error || e.message))
  } finally {
    actionLoading.value = ''
  }
}

async function resumeClient(ip: string) {
  actionLoading.value = 'resume:' + ip
  try {
    await api.resumeClient(ip)
    await loadClients()
  } catch (e: any) {
    alert('Error: ' + (e.response?.data?.error || e.message))
  } finally {
    actionLoading.value = ''
  }
}

async function removeClient(ip: string) {
  if (!confirm('Remove client ' + ip + '?')) return
  actionLoading.value = 'remove:' + ip
  try {
    await api.deleteClient(ip)
    await loadClients()
  } catch (e: any) {
    alert('Error: ' + (e.response?.data?.error || e.message))
  } finally {
    actionLoading.value = ''
  }
}

onMounted(loadClients)
</script>

<template>
  <div class="card">
    <div class="card-title">Add Client</div>
    <div style="display: flex; gap: 0.5rem; align-items: flex-end; flex-wrap: wrap;">
      <div class="form-group" style="flex: 1; min-width: 180px; margin-bottom: 0;">
        <label>IP / CIDR</label>
        <input
          v-model="newIp"
          placeholder="192.168.50.10 or 192.168.50.0/24"
          @keyup.enter="addClient"
        />
      </div>
      <div class="form-group" style="width: 140px; margin-bottom: 0;">
        <label>Route</label>
        <select v-model="newRoute">
          <option v-for="r in routeOptions" :key="r" :value="r">{{ r }}</option>
        </select>
      </div>
      <button class="btn btn-primary" :disabled="addLoading || !newIp.trim()" @click="addClient">
        {{ addLoading ? '...' : '+ Add' }}
      </button>
    </div>
  </div>

  <div class="card">
    <div class="card-title">Clients</div>

    <div class="actions">
      <button class="btn btn-blue" :disabled="loading" @click="loadClients">
        {{ loading ? '...' : '⟳ Refresh' }}
      </button>
    </div>

    <p v-if="error" class="error-msg">{{ error }}</p>

    <table v-if="clients.length > 0">
      <thead>
        <tr>
          <th>IP</th>
          <th>Route</th>
          <th>Status</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="client in clients" :key="client.ip">
          <td>{{ client.ip }}</td>
          <td>{{ client.route }}</td>
          <td>
            <span v-if="!client.paused" class="badge badge-green">Active</span>
            <span v-else class="badge badge-grey">Paused</span>
          </td>
          <td style="display: flex; gap: 0.35rem;">
            <button
              v-if="!client.paused"
              class="btn btn-yellow"
              :disabled="!!actionLoading"
              @click="pauseClient(client.ip)"
            >
              {{ actionLoading === 'pause:' + client.ip ? '...' : 'Pause' }}
            </button>
            <button
              v-else
              class="btn btn-green"
              :disabled="!!actionLoading"
              @click="resumeClient(client.ip)"
            >
              {{ actionLoading === 'resume:' + client.ip ? '...' : 'Resume' }}
            </button>
            <button
              class="btn btn-red"
              :disabled="!!actionLoading"
              @click="removeClient(client.ip)"
            >
              {{ actionLoading === 'remove:' + client.ip ? '...' : 'Remove' }}
            </button>
          </td>
        </tr>
      </tbody>
    </table>

    <p v-else-if="!loading" style="color: #999; font-size: 0.875rem;">
      No clients configured.
    </p>
  </div>
</template>
