import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { ArrowDownToLine, Clipboard, Code2, Copy, ScanLine, WrapText } from 'lucide-react'
import { header, readBytes } from './api'
import type { Detail, Headers, Row } from './api'
import { FramesPreview, KeyValueList, SpecialPreview, TimingDetails } from './Preview'
import type { Format } from './Preview'

export const methodTone = (method: string) => ({ GET: 'green', POST: 'violet', PATCH: 'blue', PUT: 'blue', DELETE: 'red', CONNECT: 'grey' }[method] ?? 'grey')
export const statusTone = (status: number) => status >= 400 ? 'red' : status >= 300 ? 'amber' : status >= 200 ? 'green' : 'grey'
export const formatSize = (n: number) => n >= 1048576 ? `${(n / 1048576).toFixed(1)} MB` : n >= 1024 ? `${(n / 1024).toFixed(1)} KB` : `${n} B`
export const statusText = (status: number) => ({ 101: 'Switching Protocols', 200: 'OK', 201: 'Created', 204: 'No Content', 301: 'Moved Permanently', 302: 'Found', 304: 'Not Modified', 400: 'Bad Request', 401: 'Unauthorized', 403: 'Forbidden', 404: 'Not Found', 422: 'Unprocessable Content', 429: 'Too Many Requests', 500: 'Internal Server Error', 502: 'Bad Gateway', 503: 'Service Unavailable' }[status] ?? '')

export function Status({ row, long = false }: { row: Row, long?: boolean }) {
  return <span className={`status tone-${row.error ? 'red' : statusTone(row.status)}`}>
    <i />{row.status ? `${row.status}${long && statusText(row.status) ? ' ' + statusText(row.status) : ''}` : row.pending ? '进行中' : '失败'}
  </span>
}

type Side = 'request' | 'response'
type View = 'body' | 'headers' | 'query' | 'cookies' | 'frames' | 'timing' | 'trailers'

async function decodeBody(value: string | null, encoding: string, truncated: boolean) {
  if (!value) return { text: '', binary: false }
  let bytes: Uint8Array<ArrayBuffer> = readBytes(value)
  if (encoding && encoding !== 'identity') {
    if (truncated || !['gzip', 'deflate', 'br'].includes(encoding) || !window.DecompressionStream) return { text: '', binary: true }
    try {
      const stream = new Blob([bytes]).stream().pipeThrough(new DecompressionStream(encoding as CompressionFormat))
      const reader = stream.getReader()
      const preview = new Uint8Array(256 * 1024)
      let offset = 0
      try {
        while (offset < preview.length) {
          const chunk = await reader.read()
          if (chunk.done) break
          const data = chunk.value.subarray(0, preview.length - offset)
          preview.set(data, offset)
          offset += data.length
        }
      } finally {
        await reader.cancel().catch(() => undefined)
      }
      bytes = preview.subarray(0, offset)
      truncated = offset === preview.length
    } catch {
      return { text: '', binary: true }
    }
  }
  try {
    const text = new TextDecoder('utf-8', { fatal: !truncated }).decode(bytes)
    return { text, binary: /[\u0000-\u0008\u000E-\u001F]/.test(text) }
  } catch {
    return { text: '', binary: true }
  }
}

function pieces(line: string, json: boolean) {
  return json ? line.split(/("(?:[^"\\]|\\.)*"|\b(?:true|false|null)\b|-?\b\d+(?:\.\d+)?\b)/g).map((part, index, parts) => {
    let name = ''
    if (index % 2 === 1) {
      if (part.startsWith('"')) name = parts[index + 1]?.trimStart().startsWith(':') ? 'json-key' : 'json-string'
      else name = 'json-value'
    }
    return <span className={name} key={index}>{part}</span>
  }) : line
}

function Payload({ side, row, detail, copy, saveBody }: { side: Side, row: Row, detail: Detail | null, copy: (text: string) => void, saveBody?: (request: boolean) => void }) {
  const request = side === 'request'
  const body = (request ? detail?.requestBody : detail?.responseBody) ?? null
  const headers: Headers = (request ? detail?.requestHeaders : detail?.responseHeaders) ?? null
  const type = header(headers, 'content-type').split(';')[0] || (!request ? row.contentType : '')
  const encoding = header(headers, 'content-encoding').toLowerCase()
  const truncated = Boolean(request ? detail?.requestTruncated : detail?.responseTruncated)
  const [view, setView] = useState<View>(request && !body ? 'headers' : 'body')
  const [format, setFormat] = useState<Format>('text')
  const [wrap, setWrap] = useState(true)
  const [raw, setRaw] = useState(false)
  const [decoded, setDecoded] = useState<Awaited<ReturnType<typeof decodeBody>> | null>(null)
  useEffect(() => {
    let cancelled = false
    setDecoded(null)
    if (view === 'body') void decodeBody(body, encoding, truncated).then(value => {
      if (!cancelled) setDecoded(value)
    })
    return () => { cancelled = true }
  }, [body, encoding, truncated, view])
  let formatted = decoded?.text ?? ''
  let isJson = type.includes('json')
  if (!raw && formatted && !decoded?.binary && (isJson || /^[\s]*[\[{]/.test(formatted))) {
    try {
      formatted = JSON.stringify(JSON.parse(formatted), null, 2)
      isJson = true
    } catch { /* Render invalid or partial JSON as received. */ }
  }
  const tabs: [View, string][] = [['body', '正文'], ['headers', '标头']]
  if (request) tabs.push(['query', '参数'])
  tabs.push(['cookies', 'Cookies'])
  if (!request && row.kind === 'WebSocket') tabs.push(['frames', '帧'])
  if (!request && detail?.timing) tabs.push(['timing', '计时'])
  if (!request && detail?.responseTrailers && Object.keys(detail.responseTrailers).length) tabs.push(['trailers', '尾标'])
  let query: [string, string][] = []
  if (view === 'query') {
    try { query = [...new URL(row.url).searchParams.entries()] } catch { /* Invalid or empty URL. */ }
  }
  const pairs = Object.entries(headers ?? {}).flatMap(([name, values]) => values.map(value => [name, value] as [string, string]))
  const cookies = pairs.filter(([key]) => key.toLowerCase() === (request ? 'cookie' : 'set-cookie'))
    .flatMap(([key, value]) => request ? value.split(';').map(item => [item.split('=')[0].trim(), item.slice(item.indexOf('=') + 1).trim()] as [string, string]) : [[key, value] as [string, string]])
  return <div className="payload"  aria-label={request ? '请求内容' : '响应内容'}>
    <div className="payload-toolbar">
      <strong className="payload-label">{request ? '请求' : '响应'}</strong>
      <div className="payload-tabs">{tabs.map(([key, label]) =>
        <button key={key} className={view === key ? 'selected' : ''} aria-pressed={view === key} onClick={() => setView(key)}>{label}</button>)}</div>
      {view === 'body' && <div className="payload-actions">
        <select className="preview-format" aria-label="正文显示格式" value={format} onChange={event => setFormat(event.target.value as Format)}>
          <option value="text">{isJson ? 'JSON' : '文本'}</option><option value="tree">树形</option><option value="hex">Hex</option>
          <option value="raw">Raw</option><option value="protocol">协议</option><option value="image">图片</option>
        </select>
        <button className="btn icon-button" aria-label="切换原始内容" title="格式化 / 原始内容" aria-pressed={raw} onClick={() => setRaw(!raw)}><Code2 size={13} /></button>
        <button className="btn icon-button" aria-label="自动换行" title="自动换行" aria-pressed={wrap} onClick={() => setWrap(!wrap)}><WrapText size={13} /></button>
        {saveBody && <button className="btn icon-button" aria-label="保存正文" title="保存正文预览" disabled={!body} onClick={() => saveBody(request)}><ArrowDownToLine size={13} /></button>}
        <button className="btn icon-button" aria-label="复制正文" title="复制正文" disabled={!formatted || decoded?.binary} onClick={() => copy(formatted)}><Clipboard size={12} /></button>
      </div>}
    </div>
    {view === 'headers' && <div className="payload-scroll"><KeyValueList pairs={pairs} empty={detail ? '无标头' : '读取标头…'} /></div>}
    {view === 'query' && <div className="payload-scroll"><KeyValueList pairs={query} empty="无查询参数" /></div>}
    {view === 'cookies' && <div className="payload-scroll"><KeyValueList pairs={cookies} empty="无 Cookies" /></div>}
    {view === 'frames' && <div className="payload-scroll"><FramesPreview frames={detail?.frames ?? []} count={row.frameCount ?? 0} copy={copy} /></div>}
    {view === 'trailers' && <div className="payload-scroll"><KeyValueList pairs={Object.entries(detail?.responseTrailers ?? {}).flatMap(([name, values]) => values.map(value => [name, value] as [string, string]))} empty="无尾标" /></div>}
    {view === 'timing' && <div className="payload-scroll"><TimingDetails detail={detail} /></div>}
    {view === 'body' && <>
      {truncated && <div className="body-notice">正文超过 64 KiB，仅保留预览。</div>}
      {format !== 'text' ? <div className="payload-scroll"><SpecialPreview format={format} bytes={body} text={decoded?.text ?? ''} mime={type} headers={headers} body={body} row={row} detail={detail} request={request} /></div>
        : decoded?.binary ? <div className="body-empty">二进制、压缩或非 UTF-8 内容<br /><button className="btn small-button" onClick={() => copy(body ?? '')}>复制 Base64</button></div>
        : formatted ? <div className={`payload-scroll code-scroll ${wrap ? 'wrap' : ''}`}><pre aria-label="正文内容">{formatted.split('\n').map((line, index) =>
          <span className="code-line" key={index}><span aria-hidden="true" className="line-number">{index + 1}</span><span>{pieces(line, isJson)}</span></span>)}</pre></div>
          : <div className="body-empty">{row.kind === 'Tunnel' ? '加密流量直通' : request && !detail || !request && row.pending ? '等待读取…' : !detail || !decoded ? '读取正文…' : '无正文'}</div>}
    </>}
  </div>
}

function curlCommand(row: Row, detail: Detail) {
  const quote = (text: string) => "'" + text.replaceAll("'", "'\\''") + "'"
  const omitted = new Set(['connection', 'content-length', 'host', 'transfer-encoding', 'proxy-authorization', 'proxy-connection', 'upgrade'])
  const args = ['curl', '-X', quote(row.method), quote(row.url)]
  for (const [name, values] of Object.entries(detail.requestHeaders ?? {})) {
    if (!omitted.has(name.toLowerCase())) for (const value of values) args.push('-H', quote(`${name}: ${value}`))
  }
  if (detail.requestBody) {
    const bytes = readBytes(detail.requestBody)
    let text = ''
    try { text = new TextDecoder('utf-8', { fatal: true }).decode(bytes) } catch { /* Use the base64 pipeline for binary data. */ }
    if (!text || text.includes('\0') || header(detail.requestHeaders, 'content-encoding')) {
      return `printf %s ${quote(detail.requestBody)} | base64 -d | ${args.join(' ')} --data-binary @-`
    }
    args.push('--data-binary', quote(text))
  }
  return args.join(' ')
}

export function Inspector({ row, detail, dock, copy, tools, saveBody }: { row?: Row, detail: Detail | null, dock: 'right' | 'bottom', copy: (text: string) => void, tools?: ReactNode, saveBody?: (request: boolean) => void }) {
  const [side, setSide] = useState<Side>('response')
  if (!row) return <aside className="inspector inspector-empty" aria-label="请求详情">
    <div className="summary-strip"><ScanLine size={14} /> 请求详情</div>
    <div className="empty-detail"><ScanLine size={22} /><span>选择请求以查看详情</span></div>
  </aside>

  return <aside className="inspector" aria-label="请求详情">
    <div className="summary-strip">
      <span className={`badge method tone-${methodTone(row.method)}`}>{row.method}</span>
      <Status row={row} long />
      <span className="summary-url mono" title={row.url}>{row.url}</span>
      {row.source && <span className="source-tag">{row.source}</span>}
      <span className="summary-metrics mono"><span className="summary-protocol">{detail?.protocol || row.kind}</span><span>{row.duration} ms</span></span>
      <button className="btn icon-button" title="复制请求 URL" aria-label="复制请求 URL" onClick={() => copy(row.url)}><Copy size={13} /></button>
      <button className="btn curl-button" disabled={!detail || row.method === 'CONNECT' || row.pending || detail.requestTruncated}
        title={detail?.requestTruncated ? '请求体已截断' : '复制为 cURL（POSIX Shell）'} onClick={() => { if (detail) copy(curlCommand(row, detail)) }}>cURL</button>
    </div>
    {tools}
    {detail?.scriptError && <div className="request-error" role="status">脚本错误 · {detail.scriptError}</div>}
    {row.error && <div className="request-error" role="status">{row.error}</div>}
    {dock === 'right' && <div className="inspector-tabs" aria-label="详情视图">
      {([['request', '请求'], ['response', '响应']] as const).map(([key, label]) =>
        <button key={key} className={side === key ? 'selected' : ''} aria-pressed={side === key} onClick={() => setSide(key)}>{label}</button>)}
    </div>}
    <div className="payload-grid">
      {(dock === 'bottom' ? ['request', 'response'] as const : [side]).map(value =>
        <Payload key={value} side={value} row={row} detail={detail} copy={copy} saveBody={saveBody} />)}
    </div>
  </aside>
}
