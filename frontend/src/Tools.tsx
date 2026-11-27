import { useMemo, useState } from 'react'
import { ArrowUp, ChevronDown, CirclePlus, Play, Trash2 } from 'lucide-react'
import { readBytes } from './api'
import type { BreakpointAction, Condition, Detail, Headers, NetworkOptions, RequestInput, Row, Rule } from './api'

export function textOf(value: string | null | undefined) {
  if (!value) return ''
  try { return new TextDecoder('utf-8', { fatal: true }).decode(readBytes(value)) } catch { return '' }
}

function parseHeaders(lines: string) {
  const result: Record<string, string[]> = Object.create(null)
  for (const line of lines.split('\n')) {
    if (!line.trim()) continue
    const split = line.indexOf(':')
    if (split <= 0) throw new Error('每行标头应为 名称: 值')
    const key = line.slice(0, split).trim()
    const value = line.slice(split + 1).trim()
    ;(result[key] ??= []).push(value)
  }
  return result
}

function headerText(headers: Headers) {
  return Object.entries(headers ?? {}).flatMap(([key, values]) => values.map(value => `${key}: ${value}`)).join('\n')
}

export function ComposePanel({ initial, pending, submit, close }: { initial: Detail | null, pending: boolean, submit: (input: () => RequestInput) => void, close: () => void }) {
  const [method, setMethod] = useState(initial?.method ?? 'GET')
  const [url, setURL] = useState(initial?.url ?? '')
  const [headers, setHeaders] = useState(headerText(initial?.requestHeaders ?? null))
  const [body, setBody] = useState(textOf(initial?.requestBody))
  const submitInput = () => submit(() => {
    const parsed = parseHeaders(headers)
    for (const key of Object.keys(parsed)) {
      if (['content-length', 'transfer-encoding', 'content-encoding'].includes(key.toLowerCase())) delete parsed[key]
    }
    return { method, url, headers: parsed, body }
  })
  return <form onSubmit={event => { event.preventDefault(); submitInput() }}>
    <div className="dialog-section tool-fields">
      <label className="field-label" htmlFor="compose-url">请求地址</label>
      <div className="field-line"><select className="text-input mono" aria-label="HTTP 方法" value={method} onChange={event => setMethod(event.target.value)}>
        {Array.from(new Set(['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS', method])).map(item => <option key={item}>{item}</option>)}
      </select><input id="compose-url" autoFocus className="text-input mono stretch" type="url" required placeholder="https://api.example.test/v1/health" value={url} onChange={event => setURL(event.target.value)} /></div>
      {initial?.requestTruncated && <p>已载入截断的请求正文，请按需补全。</p>}
      {initial?.requestBody && !body && <p>原始二进制正文可使用直接重发。编辑器仅接受文本。</p>}
      <label className="field-label" htmlFor="compose-headers">标头</label>
      <textarea id="compose-headers" className="text-input mono tool-text" value={headers} onChange={event => setHeaders(event.target.value)} placeholder="Content-Type: application/json" />
      <label className="field-label" htmlFor="compose-body">正文</label>
      <textarea id="compose-body" className="text-input mono tool-text medium" value={body} onChange={event => setBody(event.target.value)} placeholder='{"name":"Peekly"}' />
    </div>
    <div className="modal-actions"><button type="button" className="btn subtle-button" onClick={close}>关闭</button><button type="submit" className="btn capture-button" disabled={pending || !url.trim()}><Play size={13} />{pending ? '正在发送…' : '发送请求'}</button></div>
  </form>
}

export const ruleKinds: Record<string, string> = {
  mock: '模拟响应', file: '本地文件映射', directory: '本地目录映射', redirect: '远程映射',
  'break-request': '请求断点', 'break-response': '响应断点', block: '阻止', allow: '允许列表', bypass: '绕过抓包',
  'request-set': '覆盖请求标头', 'request-add': '追加请求标头', 'request-remove': '删除请求标头',
  'response-set': '覆盖响应标头', 'response-add': '追加响应标头', 'response-remove': '删除响应标头',
  delay: '模拟延迟', throttle: '响应限速', 'no-cache': '禁用缓存',
}

export function RulesPanel({ initial, pending, submit, close }: { initial: Rule[], pending: boolean, submit: (rules: Rule[]) => void, close: () => void }) {
  const [rules, setRules] = useState(initial)
  const [expanded, setExpanded] = useState(initial.length ? 0 : -1)
  function patch(index: number, value: Partial<Rule>) {
    setRules(rules.map((rule, i) => i === index ? { ...rule, ...value } : rule))
  }
  function add() {
    setExpanded(rules.length)
    setRules([...rules, { enabled: true, kind: 'mock', match: 'contains', pattern: '', name: '', value: '', number: 200 }])
  }
  return <div>
    <div className="dialog-section tool-fields">
      <p>规则按顺序匹配 URL，HTTPS 内容规则需要先为相应域名启用解密。</p>
      <div className="rules-list">{rules.map((rule, index) => {
        const header = rule.kind.startsWith('request-') || rule.kind.startsWith('response-')
        const local = rule.kind === 'file' || rule.kind === 'directory'
        const number = rule.kind === 'mock' || rule.kind === 'delay' || rule.kind === 'throttle'
        return <section className="rule-item" key={index}>
          <div className="rule-title">
            <input type="checkbox" aria-label="启用规则" checked={rule.enabled} onChange={event => patch(index, { enabled: event.target.checked })} />
            <button className="rule-expand" onClick={() => setExpanded(expanded === index ? -1 : index)} aria-expanded={expanded === index}>
              <ChevronDown size={13} /> <strong>{ruleKinds[rule.kind]}</strong><span title={rule.pattern} className="mono">{rule.pattern || '未填写匹配表达式'}</span>
            </button>
            <button className="btn icon-button" title="上移" aria-label="上移规则" disabled={index === 0} onClick={() => {
              const next = [...rules]; [next[index - 1], next[index]] = [next[index], next[index - 1]]
              setRules(next); setExpanded(index - 1)
            }}><ArrowUp size={13} /></button>
            <button className="btn icon-button" title="移除规则" aria-label="移除规则" onClick={() => { setRules(rules.filter((_, i) => i !== index)); setExpanded(-1) }}><Trash2 size={13} /></button>
          </div>
          {expanded === index && <div className="rule-edit">
            <div className="field-line"><select className="text-input" aria-label="规则动作" value={rule.kind} onChange={event => patch(index, { kind: event.target.value, name: '', value: '', number: event.target.value === 'mock' ? 200 : 100 })}>
              {Object.entries(ruleKinds).map(([key, title]) => <option key={key} value={key}>{title}</option>)}
            </select><select className="text-input" aria-label="URL 匹配方式" value={rule.match} onChange={event => patch(index, { match: event.target.value })}>
              <option value="contains">包含</option><option value="glob">通配符</option><option value="regex">正则</option>
            </select><input className="text-input mono stretch" aria-label="URL 匹配表达式" placeholder="api.example.test/v1/" value={rule.pattern} onChange={event => patch(index, { pattern: event.target.value })} /></div>
            {number && <label className="inline-label">{rule.kind === 'mock' ? '状态码' : rule.kind === 'delay' ? '延迟（ms）' : '速率（KiB/s）'}
              <input className="text-input mono" type="number" value={rule.number} onChange={event => patch(index, { number: Number(event.target.value) })} /></label>}
            {(header || rule.kind === 'mock' || rule.kind === 'directory') && <input className="text-input mono" aria-label="标头名称、MIME 或路径前缀"
              placeholder={header ? '标头名称，如 X-Debug' : rule.kind === 'directory' ? 'URL 路径前缀，如 /assets' : 'Content-Type（可选）'} value={rule.name} onChange={event => patch(index, { name: event.target.value })} />}
            {rule.kind === 'mock' ? <textarea className="text-input tool-text mono" aria-label="模拟响应正文" placeholder="模拟响应正文" value={rule.value} onChange={event => patch(index, { value: event.target.value })} />
              : (local || rule.kind === 'redirect' || header && !rule.kind.endsWith('-remove')) && <input className="text-input mono" value={rule.value}
                aria-label="映射目标或标头值" placeholder={local ? '本地绝对路径' : rule.kind === 'redirect' ? 'https://staging.example.test/v1/' : '标头值'} onChange={event => patch(index, { value: event.target.value })} />}
          </div>}
        </section>
      })}</div>
      <button className="btn subtle-button" onClick={add} disabled={rules.length >= 100}><CirclePlus size={13} /> 添加规则</button>
    </div>
    <div className="modal-actions"><button className="btn subtle-button" onClick={close}>关闭</button><button className="btn capture-button" disabled={pending} onClick={() => submit(rules)}>应用规则</button></div>
  </div>
}

const filterFields: Record<string, string> = { url: 'URL', host: '主机', method: '方法', status: '状态', type: '内容类型', requestHeaders: '请求标头', responseHeaders: '响应标头', requestBody: '请求正文', responseBody: '响应正文' }
const filterOps: Record<string, string> = { contains: '包含', is: '等于', not: '不包含', starts: '开头是', ends: '结尾是', regex: '正则' }

export function FilterPanel({ initial, any, submit, close }: { initial: Condition[], any: boolean, submit: (filters: Condition[], any: boolean) => void, close: () => void }) {
  const empty = { field: 'url', op: 'contains', value: '' }
  const [filters, setFilters] = useState(initial.length ? initial : [empty])
  const [anyMatch, setAny] = useState(any)
  const [presets, setPresets] = useState<{ name: string, filters: Condition[], any: boolean }[]>(() => {
    try { return JSON.parse(localStorage.getItem('peekly.filters') ?? '[]') } catch { return [] }
  })
  const [name, setName] = useState('')
  const patch = (index: number, update: Partial<Condition>) => setFilters(filters.map((row, i) => i === index ? { ...row, ...update } : row))
  return <div>
    <div className="dialog-section tool-fields">
      <div className="field-line"><select className="text-input" aria-label="条件匹配方式" value={anyMatch ? 'or' : 'and'} onChange={event => setAny(event.target.value === 'or')}>
        <option value="and">满足全部条件</option><option value="or">满足任一条件</option>
      </select>
      {presets.length > 0 && <select className="text-input" aria-label="载入筛选预设" value="" onChange={event => {
        const selected = presets.find(item => item.name === event.target.value)
        if (selected) { setFilters(selected.filters); setAny(selected.any); setName(selected.name) }
      }}><option value="" disabled>载入预设</option>{presets.map(item => <option key={item.name}>{item.name}</option>)}</select>}</div>
      {filters.map((item, index) => <div className="field-line" key={index}>
        <select className="text-input" value={item.field} onChange={event => patch(index, { field: event.target.value })} aria-label="筛选字段">{Object.entries(filterFields).map(([key, title]) => <option key={key} value={key}>{title}</option>)}</select>
        <select className="text-input" value={item.op} onChange={event => patch(index, { op: event.target.value })} aria-label="筛选操作">{Object.entries(filterOps).map(([key, title]) => <option key={key} value={key}>{title}</option>)}</select>
        <input className="text-input stretch mono" aria-label="筛选值" placeholder="输入筛选值" value={item.value} onChange={event => patch(index, { value: event.target.value })} />
        <button className="btn icon-button" aria-label="移除条件" onClick={() => setFilters(filters.filter((_, i) => i !== index))}><Trash2 size={13} /></button>
      </div>)}
      <button className="btn subtle-button" disabled={filters.length >= 20} onClick={() => setFilters([...filters, empty])}><CirclePlus size={13} /> 添加条件</button>
    </div>
    <div className="dialog-section field-line"><input className="text-input stretch" aria-label="筛选预设名称" placeholder="预设名称" value={name} onChange={event => setName(event.target.value)} />
      <button className="btn subtle-button" disabled={!name.trim() || !filters.some(item => item.value.trim())} onClick={() => {
        const next = [...presets.filter(item => item.name !== name.trim()).slice(-29), { name: name.trim(), filters, any: anyMatch }]
        setPresets(next)
        localStorage.setItem('peekly.filters', JSON.stringify(next))
      }}>保存预设</button></div>
    <div className="modal-actions"><button className="btn subtle-button" onClick={() => submit([], false)}>清除条件</button><button className="btn subtle-button" onClick={close}>取消</button><button className="btn capture-button" onClick={() => submit(filters.filter(item => item.value.trim()), anyMatch)}>应用筛选</button></div>
  </div>
}

export function BreakpointPanel({ detail, pending, submit, close }: { detail: Detail, pending: boolean, submit: (factory: () => BreakpointAction) => void, close: () => void }) {
  const response = detail.breakpoint === 'response'
  const [headers, setHeaders] = useState('')
  const [replaceBody, setReplaceBody] = useState(false)
  const [body, setBody] = useState(textOf(response ? detail.responseBody : detail.requestBody))
  const [status, setStatus] = useState(response ? detail.status : 0)
  return <div>
    <div className="dialog-section tool-fields">
      <p className="mono">{detail.method} {detail.url}</p>
      {response && <label className="inline-label">状态码<input className="text-input mono" type="number" min={200} max={599} value={status} onChange={event => setStatus(Number(event.target.value))} /></label>}
      <label htmlFor="break-headers" className="field-label">覆盖标头</label><textarea className="text-input tool-text mono" id="break-headers" placeholder="X-Debug: true" value={headers} onChange={event => setHeaders(event.target.value)} />
      <label className="inline-label"><input type="checkbox" checked={replaceBody} onChange={event => setReplaceBody(event.target.checked)} /> 替换正文</label>
      {replaceBody && <textarea className="text-input tool-text medium mono" aria-label="替换正文" value={body} onChange={event => setBody(event.target.value)} />}
    </div>
    <div className="modal-actions"><button className="btn subtle-button" onClick={close}>关闭</button><button className="btn subtle-button" disabled={pending} onClick={() => submit(() => ({ action: 'drop', status: 0, replaceBody: false, headers: {}, body: '' }))}>中止</button>
      <button className="btn capture-button" disabled={pending} onClick={() => submit(() => ({ action: 'continue', status, replaceBody, headers: parseHeaders(headers), body }))}>继续</button></div>
  </div>
}

function comparisonText(detail: Detail | null, view: string) {
  if (!detail) return ''
  if (view === 'headers') return headerText(detail.responseHeaders).split('\n').sort().join('\n')
  const text = textOf(view === 'request' ? detail.requestBody : detail.responseBody)
  try { return JSON.stringify(JSON.parse(text), null, 2) } catch { return text }
}

function compareLines(first: string, second: string) {
  const a = first.slice(0, 30000).split('\n').slice(0, 280)
  const b = second.slice(0, 30000).split('\n').slice(0, 280)
  const lengths = Array.from({ length: a.length + 1 }, () => new Uint16Array(b.length + 1))
  for (let i = a.length - 1; i >= 0; i--) for (let j = b.length - 1; j >= 0; j--)
    lengths[i][j] = a[i] === b[j] ? lengths[i + 1][j + 1] + 1 : Math.max(lengths[i + 1][j], lengths[i][j + 1])
  const diff: { left?: string, right?: string, changed: boolean }[] = []
  for (let i = 0, j = 0; i < a.length || j < b.length;) {
    if (i < a.length && j < b.length && a[i] === b[j]) diff.push({ left: a[i++], right: b[j++], changed: false })
    else if (j >= b.length || i < a.length && lengths[i + 1][j] >= lengths[i][j + 1]) diff.push({ left: a[i++], changed: true })
    else diff.push({ right: b[j++], changed: true })
  }
  return diff
}

export function DiffPanel({ base, rows, fetchDetail }: { base: Detail, rows: Row[], fetchDetail: (id: number) => Promise<Detail> }) {
  const [other, setOther] = useState<Detail | null>(null)
  const [view, setView] = useState('response')
  const [target, setTarget] = useState(0)
  const [problem, setProblem] = useState('')
  const diff = useMemo(() => other ? compareLines(comparisonText(base, view), comparisonText(other, view)) : [], [base, other, view])
  return <div className="dialog-section tool-fields">
    <div className="field-line"><strong className="mono">基准 #{base.id}</strong><select className="text-input stretch" aria-label="选择对比请求" value={target || ''} onChange={event => {
      const id = Number(event.target.value)
      setTarget(id); setOther(null); setProblem('')
      void fetchDetail(id).then(value => { setOther(current => current?.id === id ? current : value) }).catch(error => setProblem(String(error)))
    }}><option value="" disabled>选择对比请求</option>{rows.filter(item => item.id !== base.id && !item.pending).map(item => <option key={item.id} value={item.id}>#{item.id} {item.method} {item.path}</option>)}</select>
    <select className="text-input" aria-label="对比内容" value={view} onChange={event => setView(event.target.value)}><option value="response">响应正文</option><option value="request">请求正文</option><option value="headers">响应标头</option></select></div>
    {problem && <p role="alert">{problem}</p>}
    {other?.id === target && <>
      <div className="diff-summary mono"><span>#{base.id} · {base.status} · {base.duration} ms</span><span>#{other.id} · {other.status} · {other.duration} ms</span></div>
      <div className="diff-code mono">{diff.map(({ left, right, changed }, index) => <div key={index}>
        <span className={changed && left !== undefined ? 'diff-removed' : ''}>{left ?? ''}</span><span className={changed && right !== undefined ? 'diff-added' : ''}>{right ?? ''}</span>
      </div>)}</div>
      <p>对比显示各侧正文或标头的前 280 行，最长 30,000 字符。</p>
    </>}
  </div>
}

export function NetworkPanel({ initial, addresses, pending, submit, close }: {
  initial: NetworkOptions, addresses: string[], pending: boolean, submit: (settings: () => NetworkOptions) => void, close: () => void
}) {
  const [bind, setBind] = useState(initial.bind || '127.0.0.1')
  const [clients, setClients] = useState((initial.clients ?? []).join('\n'))
  const [upstream, setUpstream] = useState(initial.upstream)
  const [reverse, setReverse] = useState(initial.reverse)
  const [dns, setDNS] = useState(Object.entries(initial.dns ?? {}).map(([host, ip]) => `${host} = ${ip}`).join('\n'))
  function save() {
    submit(() => {
      const mapping: Record<string, string> = Object.create(null)
      for (const line of dns.split('\n').map(item => item.trim()).filter(Boolean)) {
        const [host, ip, rest] = line.split('=').map(item => item.trim())
        if (!host || !ip || rest !== undefined) throw new Error('DNS 映射每行应为 域名 = IP')
        mapping[host] = ip
      }
      return { bind, clients: clients.split(/[\s,]+/).filter(Boolean), upstream: upstream.trim(), reverse: reverse.trim(), dns: mapping }
    })
  }
  return <>
    <div className="dialog-section tool-fields">
      <label className="field-label" htmlFor="bind">监听地址</label><select className="text-input mono" id="bind" value={bind} onChange={event => setBind(event.target.value)}>
        {[...new Set([...addresses, bind])].map(ip => <option key={ip}>{ip}</option>)}</select>
      {bind !== '127.0.0.1' && <><label className="field-label" htmlFor="clients">允许连接的设备 IP 或子网</label>
        <textarea id="clients" className="tool-text text-input mono" placeholder="192.168.1.42" value={clients} onChange={event => setClients(event.target.value)} /></>}
      <label className="field-label" htmlFor="upstream">上游代理</label><input id="upstream" className="text-input mono" placeholder="http://127.0.0.1:8080" value={upstream} onChange={event => setUpstream(event.target.value)} />
      <label className="field-label" htmlFor="reverse">反向代理目标</label><input id="reverse" className="text-input mono" placeholder="http://localhost:3000/" value={reverse} onChange={event => setReverse(event.target.value)} />
      <label className="field-label" htmlFor="dns">本地 DNS 映射</label><textarea id="dns" className="tool-text text-input mono" placeholder="app.example.test = 127.0.0.1" value={dns} onChange={event => setDNS(event.target.value)} />
      <p>留空可关闭该设置。网络设置仅在当前运行期间有效。上游代理支持 HTTP、HTTPS 和 SOCKS5。</p>
    </div>
    <div className="modal-actions"><button className="btn subtle-button" onClick={close}>关闭</button><button className="btn capture-button" disabled={pending} onClick={save}>应用设置</button></div>
  </>
}
