export interface Server {
  name: string
  address: string
  port: number
  uuid: string
  ips: string[]
}

export interface ClientInfo {
  ip: string
  route: string
  paused: boolean
}

export interface StatusResponse {
  output: string
}

export interface VersionResponse {
  version: string
  commit: string
}

export interface UpdateCheckResponse {
  current?: string
  latest?: string
  update_available: boolean
  changelog?: string
  checked_at?: string
  dev?: boolean
}

export interface UpdateStartResponse {
  ok: boolean
  from?: string
  to?: string
  update_available?: boolean
}
