export type Kind = 'HTTP' | 'HTTPS' | 'WebSocket' | 'Tunnel'
export type Headers = Record<string, string[]> | null

export interface Row {
  id: number
  started: string
  method: string
  url: string
  host: string
  path: string
  kind: Kind
  status: number
  duration: number
  size: number
  contentType: string
  pending: boolean
  error?: string
  rule?: string
  frameCount?: number
  breakpoint?: "request" | "response"
  source?: string
}

export interface Detail extends Row {
  requestHeaders: Headers
  responseHeaders: Headers
  requestBody: string | null
  responseBody: string | null
  requestBytes: number
  requestTruncated: boolean
  responseTruncated: boolean
  protocol: string
  responseProtocol?: string
  responseTrailers?: Headers
  timing?: Timing
  frames?: Frame[]
}

export interface Timing { dns: number, connect: number, tls: number, wait: number, receive: number, remote: string, reused: boolean }
export interface Frame { direction: 'send' | 'receive', opcode: number, data: string | null, size: number, final: boolean, time: string, compressed: boolean }

export interface Snapshot {
  running: boolean
  endpoint: string
  rules: string[]
  revision: number
  changed: boolean
  rows: Row[] | null
  systemProxy: { active: boolean, pending: boolean }
}

export interface Certificate {
  fingerprint: string
}

export interface NetworkOptions { bind: string, clients: string[] | null, upstream: string, reverse: string, dns: Record<string, string> | null }

export interface Rule {
  enabled: boolean
  kind: string
  match: string
  pattern: string
  name: string
  value: string
  number: number
}
export interface Condition { field: string, op: string, value: string }
export interface RequestInput { method: string, url: string, headers: Record<string, string[]>, body: string }
export interface BreakpointAction { action: 'continue' | 'drop', replaceBody: boolean, body: string, status: number, headers: Record<string, string[]> }

export const native = '_wails' in window

async function call<T>(name: string, ...args: unknown[]): Promise<T> {
  if (!native) throw new Error('请在 Peekly 桌面应用中使用实时抓包。')
  const { Call } = await import('@wailsio/runtime')
  return Call.ByName(`main.Desktop.${name}`, ...args) as Promise<T>
}

export const api = {
  snapshot: (revision = 0) => call<Snapshot>('Snapshot', revision),
  start: (port: number, automatic: boolean) => call<string>('Start', port, automatic),
  stop: () => call<void>('Stop'),
  restoreSystemProxy: () => call<void>('RestoreSystemProxy'),
  clear: () => call<void>('Clear'),
  detail: (id: number) => call<Detail>('Detail', id),
  rules: (rules: string[]) => call<string[]>('Rules', rules),
  platform: () => call<string>('Platform'),
  certificate: () => call<Certificate>('Certificate'),
  exportCertificate: () => call<boolean>('ExportCertificate'),
  exportHAR: (ids: number[], redact: boolean) => call<boolean>('ExportHAR', ids, redact),
  importHAR: () => call<number>('ImportHAR'),
  saveBody: (id: number, request: boolean) => call<boolean>('SaveBody', id, request),
  compose: (input: RequestInput) => call<Detail>('Compose', input),
  repeat: (id: number) => call<Detail>('Repeat', id),
  network: () => call<NetworkOptions>('Network'),
  saveNetwork: (settings: NetworkOptions) => call<void>('SaveNetwork', settings),
  localAddresses: () => call<string[]>('LocalAddresses'),
  trafficRules: () => call<Rule[]>('TrafficRules'),
  saveTrafficRules: (rules: Rule[]) => call<void>('SaveTrafficRules', rules),
  matchTraffic: (conditions: Condition[], anyMatch: boolean) => call<number[]>('MatchTraffic', conditions, anyMatch),
  resolveBreakpoint: (id: number, action: BreakpointAction) => call<void>('ResolveBreakpoint', id, action),
  async copy(text: string) {
    if (native) {
      if (!await call<boolean>('Copy', text)) throw new Error('无法写入剪贴板。')
    } else {
      await navigator.clipboard.writeText(text)
    }
  },
}

export function readBytes(value: string | null) {
  return Uint8Array.from(atob(value || ''), character => character.charCodeAt(0))
}

export function header(headers: Headers, name: string) {
  return Object.entries(headers ?? {}).find(([key]) => key.toLowerCase() === name)?.[1]?.[0] ?? ''
}
