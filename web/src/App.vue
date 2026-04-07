<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from './api'
import LoginPage from './components/LoginPage.vue'
import StatusTab from './components/StatusTab.vue'
import ServersTab from './components/ServersTab.vue'
import ClientsTab from './components/ClientsTab.vue'
import ExclusionsTab from './components/ExclusionsTab.vue'
import LogsTab from './components/LogsTab.vue'
import SettingsTab from './components/SettingsTab.vue'

const authenticated = ref(false)
const activeTab = ref('status')
const version = ref('')

const tabs = [
  { id: 'status', label: 'Status' },
  { id: 'servers', label: 'Servers' },
  { id: 'clients', label: 'Clients' },
  { id: 'exclusions', label: 'Exclusions' },
  { id: 'logs', label: 'Logs' },
  { id: 'settings', label: 'Settings' },
]

async function checkAuth() {
  try {
    const resp = await api.checkAuth()
    version.value = resp.data.version || ''
    authenticated.value = true
  } catch {
    authenticated.value = false
  }
}

function onLogin() {
  checkAuth()
}

async function logout() {
  try {
    await api.logout()
  } catch {
    // ignore
  }
  authenticated.value = false
}

onMounted(() => {
  checkAuth()
})
</script>

<template>
  <LoginPage v-if="!authenticated" @login="onLogin" />
  <template v-else>
    <div class="topbar">
      <div class="topbar-title">VPN Director</div>
      <div class="topbar-info">
        <span v-if="version">v{{ version }}</span>
        <button class="btn btn-red" @click="logout">Logout</button>
      </div>
    </div>
    <div class="tabs">
      <div
        v-for="tab in tabs"
        :key="tab.id"
        class="tab"
        :class="{ active: activeTab === tab.id }"
        @click="activeTab = tab.id"
      >
        {{ tab.label }}
      </div>
    </div>
    <div class="content">
      <StatusTab v-if="activeTab === 'status'" />
      <ServersTab v-if="activeTab === 'servers'" />
      <ClientsTab v-if="activeTab === 'clients'" />
      <ExclusionsTab v-if="activeTab === 'exclusions'" />
      <LogsTab v-if="activeTab === 'logs'" />
      <SettingsTab v-if="activeTab === 'settings'" />
    </div>
  </template>
</template>
