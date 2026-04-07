<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api'

const countrySets = ref<string[]>([])
const excludeIPs = ref<string[]>([])
const loading = ref(false)
const error = ref('')

const newCountry = ref('')
const countryLoading = ref(false)

const newIP = ref('')
const ipLoading = ref(false)

async function loadData() {
  loading.value = true
  error.value = ''
  try {
    const [setsRes, ipsRes] = await Promise.all([
      api.getExcludeSets(),
      api.getExcludeIPs(),
    ])
    countrySets.value = setsRes.data.sets ?? []
    excludeIPs.value = ipsRes.data.ips ?? []
  } catch (e: any) {
    error.value = e.response?.data?.error || e.message
  } finally {
    loading.value = false
  }
}

async function addCountry() {
  const code = newCountry.value.trim().toUpperCase()
  if (!/^[A-Z]{2}$/.test(code)) {
    alert('Please enter a valid 2-letter country code (e.g. US, DE, JP)')
    return
  }
  if (countrySets.value.includes(code)) {
    alert('Country code already added')
    return
  }
  countryLoading.value = true
  try {
    await api.updateExcludeSets([...countrySets.value, code])
    newCountry.value = ''
    await loadData()
  } catch (e: any) {
    alert('Error: ' + (e.response?.data?.error || e.message))
  } finally {
    countryLoading.value = false
  }
}

async function removeCountry(code: string) {
  countryLoading.value = true
  try {
    await api.updateExcludeSets(countrySets.value.filter((c) => c !== code))
    await loadData()
  } catch (e: any) {
    alert('Error: ' + (e.response?.data?.error || e.message))
  } finally {
    countryLoading.value = false
  }
}

async function addIP() {
  const ip = newIP.value.trim()
  if (!ip) return
  ipLoading.value = true
  try {
    await api.addExcludeIP(ip)
    newIP.value = ''
    await loadData()
  } catch (e: any) {
    alert('Error: ' + (e.response?.data?.error || e.message))
  } finally {
    ipLoading.value = false
  }
}

async function removeIP(ip: string) {
  ipLoading.value = true
  try {
    await api.deleteExcludeIP(ip)
    await loadData()
  } catch (e: any) {
    alert('Error: ' + (e.response?.data?.error || e.message))
  } finally {
    ipLoading.value = false
  }
}

onMounted(loadData)
</script>

<template>
  <p v-if="error" class="error-msg">{{ error }}</p>

  <div class="actions">
    <button class="btn btn-blue" :disabled="loading" @click="loadData">
      {{ loading ? '...' : '⟳ Refresh' }}
    </button>
  </div>

  <div class="grid-2">
    <!-- Country Exclusions -->
    <div class="card">
      <div class="card-title">Country Exclusions</div>

      <div style="display: flex; gap: 0.5rem; margin-bottom: 0.75rem;">
        <input
          v-model="newCountry"
          placeholder="Country code (e.g. US)"
          maxlength="2"
          style="width: 140px; text-transform: uppercase;"
          @keyup.enter="addCountry"
        />
        <button class="btn btn-primary" :disabled="countryLoading" @click="addCountry">
          {{ countryLoading ? '...' : '+ Add' }}
        </button>
      </div>

      <div v-if="countrySets.length > 0" style="display: flex; flex-wrap: wrap; gap: 0.35rem;">
        <span
          v-for="code in countrySets"
          :key="code"
          class="badge badge-green"
          style="cursor: pointer; font-size: 0.85rem;"
          @click="removeCountry(code)"
        >
          {{ code }} ✕
        </span>
      </div>
      <p v-else style="color: #999; font-size: 0.875rem;">No country exclusions.</p>
    </div>

    <!-- IP Exclusions -->
    <div class="card">
      <div class="card-title">IP Exclusions</div>

      <div style="display: flex; gap: 0.5rem; margin-bottom: 0.75rem;">
        <input
          v-model="newIP"
          placeholder="IP or CIDR (e.g. 1.2.3.4/24)"
          style="flex: 1;"
          @keyup.enter="addIP"
        />
        <button class="btn btn-primary" :disabled="ipLoading || !newIP.trim()" @click="addIP">
          {{ ipLoading ? '...' : '+ Add' }}
        </button>
      </div>

      <div v-if="excludeIPs.length > 0">
        <div
          v-for="ip in excludeIPs"
          :key="ip"
          style="display: flex; justify-content: space-between; align-items: center; padding: 0.3rem 0; border-bottom: 1px solid #2a2a3a;"
        >
          <span style="font-size: 0.875rem;">{{ ip }}</span>
          <span
            style="color: #ff6b6b; cursor: pointer; font-size: 0.85rem; padding: 0 0.25rem;"
            @click="removeIP(ip)"
          >
            ✕
          </span>
        </div>
      </div>
      <p v-else style="color: #999; font-size: 0.875rem;">No IP exclusions.</p>
    </div>
  </div>
</template>
