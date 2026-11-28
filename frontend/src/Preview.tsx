import { useMemo, useState } from 'react'
import { header, readBytes } from './api'
import type { Detail, Frame, Headers, Row } from './api'
import { SelectField } from './SelectField'

export function KeyValueList({ pairs, empty }: { pairs: [string, string][], empty: string }) {
  return pairs.length ? <dl className="key-value-list">{pairs.map(([key, value], index) =>
    <div key={index}><dt>{key}</dt><dd>{value}</dd></div>)}</dl> : <div className="body-empty">{empty}</div>
}

export function TimingDetails({ detail }: { detail: Detail | null }) {
  const timing = detail?.timing
  if (!timing) return <div className="body-empty">当前记录没有连接计时</div>
  const phases: [string, number][] = [['DNS', timing.dns], ['TCP', timing.connect], ['TLS', timing.tls], ['首字节', timing.wait], ['接收', timing.receive]]
  const total = Math.max(1, ...phases.map(([, value]) => value))
  return <div className="timing-preview">
    <KeyValueList pairs={[
      ['上游地址', timing.remote || '—'], ['复用连接', timing.reused ? '是' : '否'], ['上游协议', detail.responseProtocol || '—'],
      ['命中规则', detail.rule || '—'],
    ]} empty="" />
    {phases.map(([label, value]) => <div className="timing-row mono" key={label}><span>{label}</span>
      <span className="timing-track">{value >= 0 && <i style={{ width: `${Math.max(2, Math.round(100 * value / total))}%` }} />}</span>
      <span>{value >= 0 ? `${value} ms` : '—'}</span></div>)}
  </div>
}

export function FramesPreview({ frames, count, copy }: { frames: Frame[], count: number, copy: (text: string) => void }) {
  const [direction, setDirection] = useState('')
  const [query, setQuery] = useState('')
  const matches = frames.map(frame => {
    const preview = frame.opcode === 1 && !frame.compressed ? new TextDecoder().decode(readBytes(frame.data)) : hex(readBytes(frame.data))
    return { frame, preview }
  }).filter(({ frame, preview }) => (!direction || frame.direction === direction) && preview.toLowerCase().includes(query.toLowerCase()))
  const labels: Record<number, string> = { 0: 'Continuation', 1: 'Text', 2: 'Binary', 8: 'Close', 9: 'Ping', 10: 'Pong' }
  return frames.length ? <div className="frames-preview">
    <div className="field-line frame-filters">
      <SelectField aria-label="WebSocket 帧方向" value={direction} onChange={setDirection}
        options={[{ value: '', label: '全部帧' }, { value: 'send', label: '已发送' }, { value: 'receive', label: '已接收' }]} />
      <input className="text-input stretch" aria-label="搜索 WebSocket 帧" placeholder="搜索帧预览" value={query} onChange={event => setQuery(event.target.value)} />
    </div>
    {count > frames.length && <p>保留最近 {frames.length} 帧，共 {count} 帧。</p>}
    {!matches.length && <p>没有匹配的帧</p>}
    {matches.map(({ frame, preview }, index) => {
      return <details className="preview-item" key={`${frame.time}-${index}`}><summary className="mono">
        {frame.direction === 'send' ? '↑' : '↓'} {labels[frame.opcode] ?? frame.opcode} · {frame.size} B{frame.compressed ? ' · 压缩' : ''}{!frame.final ? ' · 分片' : ''}
        <time>{new Date(frame.time).toLocaleTimeString()}</time>
      </summary><pre>{preview || '无载荷'}</pre>
      <button type="button" className="btn small-button" disabled={!preview} onClick={() => copy(preview)}>复制帧预览</button></details>
    })}
  </div> : <div className="body-empty">等待 WebSocket 帧</div>
}

function hex(bytes: Uint8Array, max = 4096) {
  const lines: string[] = []
  for (let at = 0; at < Math.min(bytes.length, max); at += 16) {
    const slice = bytes.subarray(at, Math.min(at + 16, max))
    const data = Array.from(slice, value => value.toString(16).padStart(2, '0')).join(' ')
    const ascii = Array.from(slice, value => (value >= 32 && value < 127 ? String.fromCharCode(value) : '.')).join('')
    lines.push(`${at.toString(16).padStart(4, '0')}  ${data.padEnd(47)}  ${ascii}`)
  }
  if (bytes.length > max) lines.push(`仅显示前 ${max} 字节`)
  return lines.join('\n')
}

function NodeValue({ name, data, level }: { name: string, data: unknown, level: number }) {
  const [open, setOpen] = useState(level === 0)
  if (data === null || typeof data !== 'object') return <div className="tree-leaf"><span>{name}</span> {JSON.stringify(data)}</div>
  const entries = Object.entries(data)
  return <div className="tree-node">
    <button className="tree-toggle" aria-expanded={open} onClick={() => setOpen(!open)}>{open ? '▾' : '▸'} {name} <small>{Array.isArray(data) ? 'Array' : 'Object'} [{entries.length}]</small></button>
    {open && <div className="tree-children">{entries.slice(0, 200).map(([key, value]) => <NodeValue key={key} name={key} data={value} level={level + 1} />)}
      {entries.length > 200 && <span className="muted">仅显示前 200 个成员</span>}</div>}
  </div>
}

function Tree({ text }: { text: string }) {
  const [path, setPath] = useState('')
  let data: unknown
  try { data = JSON.parse(text) } catch { return <div className="body-empty">正文不是完整的 JSON</div> }
  const selector = path.trim().replace(/^\$\.?/, '').replace(/\[(\d+)\]/g, '.$1')
  let valid = true
  for (const key of selector.split('.').filter(Boolean)) {
    if (data !== null && typeof data === 'object' && Object.hasOwn(data, key)) data = (data as Record<string, unknown>)[key]
    else { valid = false; break }
  }
  return <div className="tree-preview mono"><input className="text-input mono" aria-label="查找 JSON 路径" placeholder="JSON 路径，如 $.data[0].name" value={path} onChange={event => setPath(event.target.value)} />
    {valid ? <NodeValue key={selector} name="JSON" data={data} level={0} /> : <div className="body-empty">查询路径不存在</div>}</div>
}

export type Format = 'text' | 'tree' | 'hex' | 'raw' | 'protocol' | 'image'

export function SpecialPreview({ format, bytes, text, mime, headers, body, row, detail, request }: {
  format: Format, bytes: string | null, text: string, mime: string, headers: Headers, body: string | null,
  row: Row, detail: Detail | null, request: boolean
}) {
  const rawBytes = useMemo(() => readBytes(bytes), [bytes])
  if (format === 'tree') return <Tree text={text} />
  if (format === 'hex') return rawBytes.length > 0 ? <pre className="special-code">{hex(rawBytes)}</pre> : <div className="body-empty">无正文</div>
  if (format === 'image') return body && /^image\/(png|jpeg|gif|webp|avif)$/.test(mime)
    ? <div className="image-preview"><img alt="响应图片预览" src={`data:${mime};base64,${body}`} /></div> : <div className="body-empty">当前类型不支持图片预览</div>
  if (format === 'raw') {
    const first = request ? `${row.method} ${row.path} ${detail?.protocol || ''}` : `${detail?.responseProtocol || 'HTTP'} ${row.status}`
    const entries = Object.entries(headers ?? {}).flatMap(([key, values]) => values.map(value => `${key}: ${value}`))
    return <pre className="special-code">{[first, ...entries, '', text || (body ? '[二进制或压缩正文，请查看十六进制]' : '')].join('\n')}</pre>
  }
  if (mime.startsWith('application/grpc')) {
    const frames: [string, string][] = []
    const data = new DataView(rawBytes.buffer)
    for (let offset = 0, index = 1; offset + 5 <= rawBytes.length && index < 100; index++) {
      const flag = data.getUint8(offset)
      const size = data.getUint32(offset + 1)
      const end = offset + 5 + size
      frames.push([`消息 ${index}`, `${size} B${flag ? ' · 压缩或尾部帧' : ''}${end > rawBytes.length ? ' · 预览截断' : ''}`])
      offset = end
    }
    return <KeyValueList pairs={frames} empty="等待 gRPC 消息" />
  }
  if (mime === 'text/event-stream') {
    const events = text.split(/\r?\n\r?\n/).filter(Boolean).slice(0, 100)
    return <div className="frames-preview">{events.map((event, index) => <details className="preview-item" key={index} open={index === 0}>
      <summary>事件 {index + 1} · {event.match(/^event:\s*(.+)$/m)?.[1] ?? 'message'}</summary><pre>{event}</pre>
    </details>)}</div>
  }
  const contentType = header(headers, 'content-type')
  if (mime.startsWith('multipart/')) {
    const boundary = contentType.match(/boundary=(?:"([^"]+)"|([^;\s]+))/i)
    if (boundary) {
      const parts = text.split('--' + (boundary[1] || boundary[2])).slice(1, 101)
      return <div className="frames-preview">{parts.filter(part => part.trim() && part.trim() !== '--').map((part, index) => <details className="preview-item" key={index} open={index === 0}>
        <summary>表单段 {index + 1} · {part.match(/name="([^"]+)"/)?.[1] ?? '字段'}</summary><pre>{part.trim()}</pre>
      </details>)}</div>
    }
  }
  let parsed: Record<string, unknown>
  try { parsed = JSON.parse(text) ?? {} } catch { parsed = {} }
  if (typeof parsed.query === 'string' || typeof parsed.model === 'string' || typeof parsed.jsonrpc === 'string' || mime.includes('graphql')) {
    const pairs = Object.entries(parsed).filter(([key]) => ['query', 'operationName', 'variables', 'model', 'id', 'jsonrpc', 'method', 'params', 'error', 'usage', 'tools'].includes(key))
      .map(([key, value]) => [key, typeof value === 'string' ? value : JSON.stringify(value, null, 2)] as [string, string])
    return <KeyValueList pairs={pairs} empty="无协议元数据" />
  }
  const token = text.trim().match(/^([\w-]+)\.([\w-]+)\.([\w-]+)$/)
  if (token) {
    try {
      const decoded = token.slice(1, 3).map(value => JSON.parse(new TextDecoder().decode(readBytes(value.replace(/-/g, '+').replace(/_/g, '/')))))
      return <KeyValueList pairs={decoded.map((value, index) => [index ? 'JWT 载荷' : 'JWT 标头', JSON.stringify(value, null, 2)])} empty="" />
    } catch { /* Only show valid base64url JSON. */ }
  }
  return <div className="body-empty">此正文没有可识别的协议字段。</div>
}
