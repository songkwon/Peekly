import { useEffect, useMemo, useState } from 'react'
import { Search } from 'lucide-react'
import { api } from './api'
import type { Detail, Row } from './api'
import { Inspector, Status, methodTone } from './Inspector'
import { SelectField } from './SelectField'

export function clientLabel(row: Row) {
  return row.client?.name || (row.client?.pid ? `PID ${row.client.pid}` : row.client?.address) || row.source || '未知'
}

export function SplitPane({ rows, copy, compare }: { rows: Row[], copy: (text: string) => void, compare: (detail: Detail) => void }) {
  const [query, setQuery] = useState('')
  const [protocol, setProtocol] = useState('')
  const [selected, setSelected] = useState<number>()
  const [detail, setDetail] = useState<Detail | null>(null)
  const visible = useMemo(() => {
    const terms = query.toLowerCase().split(/\s+/).filter(Boolean)
    return rows.filter(row => {
      const text = `${row.id} ${row.method} ${row.url} ${row.status} ${clientLabel(row)} ${row.client?.address ?? ''}`.toLowerCase()
      return (!protocol || row.kind === protocol || protocol === 'HTTPS' && row.kind === 'Tunnel') && terms.every(term => text.includes(term))
    })
  }, [rows, query, protocol])
  const row = visible.find(item => item.id === selected) ?? visible[0]
  const id = row?.id
  const pending = row?.pending
  const frameCount = row?.frameCount
  const status = row?.status
  const duration = row?.duration
  const size = row?.size
  useEffect(() => {
    if (id === undefined) return
    let current = true
    void api.detail(id).then(result => { if (current) setDetail(result) }).catch(() => { if (current) setDetail(null) })
    return () => { current = false }
  }, [id, pending, frameCount, status, duration, size])
  const active = detail?.id === id ? detail : null

  return <section className="split-pane" aria-label="右侧流量窗格">
    <div className="split-filter">
      <label className="input search-field"><Search size={13} /><input aria-label="搜索右侧请求" placeholder="筛选 URL、Client、状态…" value={query} onChange={event => setQuery(event.target.value)} /></label>
      <SelectField aria-label="右侧协议" value={protocol} onChange={setProtocol}
        options={[{ value: '', label: '全部协议' }, 'HTTP', 'HTTPS', 'WebSocket']} />
    </div>
    <div className="split-rows">
      <table aria-label="右侧请求列表">
        <colgroup><col className="col-id" /><col className="col-method" /><col /><col className="col-status" /></colgroup>
        <thead><tr><th>#</th><th>方法</th><th>请求</th><th>状态</th></tr></thead>
        <tbody>{visible.map(item => <tr className={`request-row ${item.id === id ? 'active-row' : ''}`} key={item.id}>
          <td className="mono">{item.id}</td>
          <td><span className={`badge method tone-${methodTone(item.method)}`}>{item.method}</span></td>
          <td><button className="request-link" title={`${clientLabel(item)} · ${item.url}`} aria-current={item.id === id || undefined}
            onClick={() => setSelected(item.id)}><strong className="mono">{item.host}{item.path}</strong></button></td>
          <td><Status row={item} /></td>
        </tr>)}</tbody>
      </table>
      {!visible.length && <p className="split-empty">没有匹配的请求</p>}
    </div>
    <Inspector key={id ?? 'empty'} row={row} detail={active} dock="bottom" copy={copy}
      tools={active && !pending && <div className="inspector-tools"><button className="btn tool-button" onClick={() => compare(active)}>对比</button></div>} />
  </section>
}
