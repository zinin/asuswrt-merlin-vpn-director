import { ref } from 'vue'

// The Settings tab is rendered behind v-if in App.vue, so switching tabs
// unmounts it. An update takes tens of seconds and the user will switch tabs,
// so its progress lives here, outside the component: coming back must show the
// same screen rather than a fresh one that asks a restarting server for its
// version.
export const updating = ref(false)
export const updateMessage = ref('')
export const updateTarget = ref('')

// Only one poller may run. Without this a remount would start a second loop
// and the page would reload twice, the second time over a half-loaded first.
let polling = false

/** claimPolling returns false when a poll loop is already running. */
export function claimPolling(): boolean {
  if (polling) {
    return false
  }
  polling = true
  return true
}

export function releasePolling(): void {
  polling = false
}
