<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import api from '../api'
import type { VersionResponse, UpdateCheckResponse } from '../types'
import {
  claimPolling,
  releasePolling,
  updateMessage,
  updateTarget,
  updating,
} from '../updateState'

const versionInfo = ref<VersionResponse | null>(null)
const updateInfo = ref<UpdateCheckResponse | null>(null)
const config = ref('')
const showConfig = ref(false)
const showChangelog = ref(false)
const loading = ref(false)
const checking = ref(false)
const error = ref('')

// checking is part of the guard: during a forced check updateInfo still holds
// the previous answer, so the button would offer to install a version the
// server is at that moment re-checking.
const canUpdate = computed(() => !!updateInfo.value?.update_available && !updating.value && !checking.value)

async function loadVersion() {
  error.value = ''
  try {
    const resp = await api.getVersion()
    versionInfo.value = resp.data
  } catch (e: any) {
    error.value = e.response?.data?.error || e.message
  }
}

async function loadUpdate(force = false) {
  error.value = ''
  // Only a check the user asked for clears the message: updateMessage is module
  // state now, and the other two callers are a remount - which must show the
  // message it left behind - and doUpdate, which overwrites it a line later.
  if (force) {
    updateMessage.value = ''
  }
  checking.value = true
  try {
    const resp = await api.checkUpdate(force)
    updateInfo.value = resp.data
  } catch (e: any) {
    error.value = e.response?.data?.error || e.message
  } finally {
    checking.value = false
  }
}

async function loadConfig() {
  error.value = ''
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

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

// waitForVersion polls /api/version until the new build answers or five
// minutes pass. Connection errors are expected: the server is restarting.
async function waitForVersion(target: string) {
  const deadline = Date.now() + 5 * 60 * 1000
  while (Date.now() < deadline) {
    await sleep(3000)
    try {
      const resp = await api.pollVersion()
      if (resp.data?.version === target) {
        return true
      }
    } catch {
      // server is down mid-restart, keep waiting
    }
  }
  return false
}

// runPolling owns the waiting screen. It survives this component: if the user
// leaves the tab the loop keeps going against the module state, and a remount
// re-renders the same screen instead of starting a second loop.
async function runPolling() {
  if (!claimPolling()) return
  try {
    updateMessage.value = 'Updating, the server is restarting...'
    const arrived = await waitForVersion(updateTarget.value)
    if (arrived) {
      // Reload so the browser picks up the new bundle.
      window.location.reload()
      return
    }
    updateMessage.value = 'The new version did not come up within 5 minutes. Check the logs.'
  } finally {
    updating.value = false
    // Both producers write the target before calling this, so clearing it here
    // - not in doUpdate - keeps a timed-out attempt from leaving a stale
    // version behind for the next attempt or for the onMounted rejoin to poll.
    updateTarget.value = ''
    releasePolling()
  }
}

async function doUpdate() {
  const target = updateInfo.value?.latest
  if (!target) return
  if (!confirm(`Update VPN Director to ${target}?`)) return

  updating.value = true
  error.value = ''
  updateMessage.value = 'Starting update...'
  try {
    const resp = await api.update()
    if (resp.data.update_available === false) {
      await loadUpdate()
      updateMessage.value = 'Already running the latest version.'
      updating.value = false
      return
    }
    updateTarget.value = resp.data.to || target
  } catch (e: any) {
    updateMessage.value = 'Error: ' + (e.response?.data?.error || e.message)
    updating.value = false
    return
  }
  await runPolling()
}

// resumeFromServer covers the case the module state cannot: a page reload, or a
// second browser, meeting an update it never started. /api/update/status is the
// only way to tell "still updating" from "done".
async function resumeFromServer() {
  try {
    const resp = await api.updateStatus()
    if (!resp.data.in_progress) return
  } catch {
    // The server is unreachable; the normal error paths already say so.
    return
  }
  if (!updateTarget.value) {
    updateTarget.value = updateInfo.value?.latest || ''
  }
  if (!updateTarget.value) return
  updating.value = true
  void runPolling()
}

onMounted(async () => {
  if (updating.value) {
    // An update started before the tab was left. Rejoin its screen instead of
    // asking a restarting server for a version and a release list.
    //
    // With no target yet there is nothing to poll for: doUpdate is still
    // awaiting /api/update, the answer that carries the version to wait for.
    // That call outlives this component - an in-flight async call keeps its
    // closure alive - so it starts the real loop itself once the answer lands.
    // Claiming the loop here would poll for '' and lock doUpdate out of it.
    if (updateTarget.value) {
      void runPolling()
    }
    return
  }
  await loadVersion()
  await loadUpdate()
  await resumeFromServer()
})
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
      <div class="kv" v-if="updateInfo && !updateInfo.dev">
        <span class="kv-label">Latest</span>
        <span>{{ updateInfo.latest || '—' }}</span>
      </div>
      <div class="kv" v-if="updateInfo?.dev">
        <span class="kv-label">Latest</span>
        <span>dev build, updates disabled</span>
      </div>
    </div>
    <p v-else style="color: #999; font-size: 0.875rem;">Loading...</p>

    <div class="actions">
      <button class="btn" :disabled="checking || updating" @click="loadUpdate(true)">
        {{ checking ? '...' : '⟳ Check for updates' }}
      </button>
      <button class="btn btn-primary" :disabled="!canUpdate" @click="doUpdate">
        {{ updating ? 'Updating...' : (updateInfo?.latest ? `⬆ Update to ${updateInfo.latest}` : '⬆ Update') }}
      </button>
      <button
        v-if="updateInfo?.changelog"
        class="btn btn-blue"
        @click="showChangelog = !showChangelog"
      >
        {{ showChangelog ? 'Hide changelog' : 'Changelog' }}
      </button>
    </div>
    <p v-if="updateMessage" style="font-size: 0.875rem;">{{ updateMessage }}</p>
    <pre
      v-if="showChangelog && updateInfo?.changelog"
      style="font-size: 12px; white-space: pre-wrap; line-height: 1.5; max-height: 300px; overflow-y: auto; background: #1a1a2e; padding: 0.75rem; border-radius: 4px; border: 1px solid #333;"
    >{{ updateInfo.changelog }}</pre>
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
