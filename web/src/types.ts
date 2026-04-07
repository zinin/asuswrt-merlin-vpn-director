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
