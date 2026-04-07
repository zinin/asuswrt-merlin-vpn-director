import axios from 'axios'

const api = axios.create({
  withCredentials: true,
})

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401 && !error.config?.url?.includes('/api/login')) {
      localStorage.removeItem('token')
      window.location.reload()
    }
    return Promise.reject(error)
  },
)

export default {
  // Auth
  login: (username: string, password: string) =>
    api.post('/api/login', { username, password }),
  logout: () =>
    api.post('/api/logout'),

  // Status & Control
  getStatus: () =>
    api.get('/api/status'),
  apply: () =>
    api.post('/api/apply'),
  restart: () =>
    api.post('/api/restart'),
  stop: () =>
    api.post('/api/stop'),
  updateIPsets: () =>
    api.post('/api/ipsets/update'),

  // Info
  getIP: () =>
    api.get('/api/ip'),
  getVersion: () =>
    api.get('/api/version'),

  // Servers
  getServers: () =>
    api.get('/api/servers'),
  selectServer: (index: number) =>
    api.post('/api/servers/active', { index }),
  importServers: (url: string) =>
    api.post('/api/servers/import', { url }),

  // Clients
  getClients: () =>
    api.get('/api/clients'),
  addClient: (ip: string, route: string) =>
    api.post('/api/clients', { ip, route }),
  pauseClient: (ip: string) =>
    api.post('/api/clients/pause', null, { params: { ip } }),
  resumeClient: (ip: string) =>
    api.post('/api/clients/resume', null, { params: { ip } }),
  deleteClient: (ip: string) =>
    api.delete('/api/clients', { params: { ip } }),

  // Exclusions
  getExcludeSets: () =>
    api.get('/api/excludes/sets'),
  updateExcludeSets: (sets: string[]) =>
    api.post('/api/excludes/sets', { sets }),
  getExcludeIPs: () =>
    api.get('/api/excludes/ips'),
  addExcludeIP: (ip: string) =>
    api.post('/api/excludes/ips', { ip }),
  deleteExcludeIP: (ip: string) =>
    api.delete('/api/excludes/ips', { params: { ip } }),

  // Logs & Config
  getLogs: (source?: string, lines?: number) =>
    api.get('/api/logs', { params: { ...(source ? { source } : {}), ...(lines ? { lines } : {}) } }),
  getConfig: () =>
    api.get('/api/config'),

  // System
  update: () =>
    api.post('/api/update'),
}
