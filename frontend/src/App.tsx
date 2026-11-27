import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { Activity, ArrowDown, ArrowDownToLine, ArrowRight, ArrowUpFromLine, Cable, Check, ChevronRight, Circle, CircleHelp, Command, Copy, FilePlus2, GitCompareArrows, ListFilter, PenLine, Repeat2, Waypoints, Globe2, LockKeyhole, Moon, Network, PanelBottom, PanelRight, Radio, Search, Settings2, ShieldCheck, Square, Sun, Trash2, X, Zap } from 'lucide-react'
import { api, native } from './api'
import type { BreakpointAction, Condition, Detail, NetworkOptions, RequestInput, Row, Rule } from './api'
import { ConnectionGuide } from './ConnectionGuide'
import { Inspector, Status, formatSize, methodTone } from './Inspector'
import { BreakpointPanel, ComposePanel, DiffPanel, FilterPanel, NetworkPanel, RulesPanel } from './Tools'

type Filter = 'all' | 'HTTP' | 'HTTPS' | 'WebSocket' | 'json' | 'errors'
type SortKey = 'id' | 'method' | 'path' | 'host' | 'status' | 'contentType' | 'started' | 'duration' | 'size'
type Dock = 'right' | 'bottom'
type Appearance = 'light' | 'dark' | 'system'
type CaptureMode = 'automatic' | 'manual'
type Dialog = 'connection' | 'tls' | 'export' | 'compose' | 'rules' | 'filter' | 'compare' | 'breakpoint' | 'network' | 'capture-mode'
const filters: { id: Filter, label: string }[] = [
  { id: 'all', label: '全部' }, { id: 'HTTP', label: 'HTTP' }, { id: 'HTTPS', label: 'HTTPS' },
  { id: 'WebSocket', label: 'WebSocket' }, { id: 'json', label: 'JSON' }, { id: 'errors', label: '错误' },
]
const columns: { id: SortKey, label: string }[] = [
  { id: 'id', label: '#' }, { id: 'method', label: '方法' }, { id: 'path', label: '路径' }, { id: 'host', label: '主机' },
  { id: 'status', label: '状态' }, { id: 'contentType', label: '类型' }, { id: 'started', label: '时间' },
  { id: 'duration', label: '耗时' }, { id: 'size', label: '响应大小' },
]

function fileType(contentType: string) {
  if (contentType.includes('json')) return 'JSON'
  if (contentType === 'text/event-stream') return 'SSE'
  if (contentType === 'text/javascript' || contentType === 'application/javascript') return 'JS'
  return contentType?.split('/').pop()?.toUpperCase() || '—'
}

const formatTime = (date: string) => {
  const time = new Date(date)
  return `${time.toLocaleTimeString('en-GB', { hour12: false })}.${String(time.getMilliseconds()).padStart(3, '0')}`
}

function Modal({ title, icon, onClose, children, notice, wide }: { title: string, icon: ReactNode, onClose: () => void, children: ReactNode, notice?: { message: string, error: boolean }, wide?: boolean }) {
  const ref = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    ref.current?.showModal()
  }, [])
  return <dialog ref={ref} className="modal peek-modal" onCancel={event => { event.preventDefault(); onClose() }} aria-labelledby="modal-title">
    <div className={`modal-box ${wide ? 'wide' : ''}`}>
      <div className="modal-title"><span>{icon}</span><h2 id="modal-title">{title}</h2><button className="btn icon-button" onClick={onClose} aria-label="关闭"><X size={18} /></button></div>
      {children}
      {notice && <div className={`dialog-notice ${notice.error ? 'dialog-notice-error' : ''}`} role={notice.error ? 'alert' : 'status'}>{notice.message}</div>}
    </div>
    <form method="dialog" className="modal-backdrop" onSubmit={event => { event.preventDefault(); onClose() }}><button aria-label="关闭"></button></form>
  </dialog>
}

export function App() {
  const [running, setRunning] = useState(false)
  const [systemProxy, setSystemProxy] = useState({ active: false, pending: false })
  const [captureMode, setCaptureMode] = useState<CaptureMode | null>(() => {
    const stored = localStorage.getItem('peekly.capture-mode')
    return stored === 'automatic' || stored === 'manual' ? stored : null
  })
  const [rows, setRows] = useState<Row[]>([])
  const [connectedAddress, setConnectedAddress] = useState('')
  const [ruleCount, setRuleCount] = useState(0)
  const [network, setNetwork] = useState<NetworkOptions>({ bind: '127.0.0.1', clients: [], upstream: '', reverse: '', dns: {} })
  const [networkAddresses, setNetworkAddresses] = useState<string[]>([])
  const [port, setPort] = useState(9090)
  const [portDraft, setPortDraft] = useState('9090')
  const [ruleDraft, setRuleDraft] = useState('')
  const [fingerprint, setFingerprint] = useState('')
  const [platform, setPlatform] = useState(navigator.userAgent.includes('Windows') ? 'windows' : 'darwin')
  const [appearance, setAppearance] = useState<Appearance>(() => {
    const saved = localStorage.getItem('peekly.appearance')
    return saved === 'dark' || saved === 'light' ? saved : 'system'
  })
  const [systemDark, setSystemDark] = useState(() => window.matchMedia('(prefers-color-scheme: dark)').matches)
  const dark = appearance === 'dark' || appearance === 'system' && systemDark
  const [dialog, setDialog] = useState<Dialog | null>(null)
  const [busy, setBusy] = useState(false)
  const [conditions, setConditions] = useState<Condition[]>([])
  const [anyMatch, setAnyMatch] = useState(false)
  const [matches, setMatches] = useState<Set<number> | null>(null)
  const [trafficRules, setTrafficRules] = useState<Rule[]>([])
  const [composeDraft, setComposeDraft] = useState<Detail | null>(null)
  const [comparison, setComparison] = useState<Detail | null>(null)
  const [breakHit, setBreakHit] = useState<Detail | null>(null)
  const [redacted, setRedacted] = useState(true)
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<Filter>('all')
  const [host, setHost] = useState('')
  const [selected, setSelected] = useState<number>()
  const [detail, setDetail] = useState<Detail | null>(null)
  const [toast, setToast] = useState<{ message: string, error: boolean }>()
  const [sort, setSort] = useState<{ key: SortKey, reverse: boolean }>({ key: 'id', reverse: true })
  const [dock, setDock] = useState<Dock>(() => localStorage.getItem('peekly.dock') === 'bottom' ? 'bottom' : 'right')
  const [desktopProblem, setDesktopProblem] = useState('')
  const search = useRef<HTMLInputElement>(null)
  const revision = useRef(0)
  const requestNumber = useRef(0)
  const rules = useRef<string[]>([])
  const toastTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  const showToast = useCallback((message: string, error = false) => {
    if (toastTimer.current) clearTimeout(toastTimer.current)
    setToast({ message, error })
    toastTimer.current = setTimeout(() => setToast(undefined), 4200)
  }, [])

  const refresh = useCallback(async () => {
    const request = ++requestNumber.current
    const state = await api.snapshot(revision.current)
    if (request !== requestNumber.current) return
    setRunning(state.running)
    setSystemProxy(state.systemProxy)
    setConnectedAddress(state.endpoint)
    rules.current = state.rules
    setRuleCount(state.rules.length)
    if (state.changed) {
      revision.current = state.revision
      setRows(state.rows ?? [])
    }
    setDesktopProblem('')
  }, [])

  useEffect(() => {
    const setting = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = (change: MediaQueryListEvent) => setSystemDark(change.matches)
    setting.addEventListener('change', onChange)
    return () => setting.removeEventListener('change', onChange)
  }, [])

  useEffect(() => {
    document.documentElement.dataset.appearance = dark ? 'dark' : 'light'
    localStorage.setItem('peekly.appearance', appearance)
  }, [dark, appearance])

  useEffect(() => {
    if (!native) return
    let timer: ReturnType<typeof setTimeout>
    let active = true
    const tick = async () => {
      try { await refresh() }
      catch (error) { if (active) setDesktopProblem(error instanceof Error ? error.message : String(error)) }
      if (active) timer = setTimeout(tick, 650)
    }
    void tick()
    void api.platform().then(setPlatform).catch(() => {})
    return () => { active = false; clearTimeout(timer) }
  }, [refresh])

  useEffect(() => {
    const shortcuts = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && (event.key.toLowerCase() === 'k' || event.key.toLowerCase() === 'f') && !dialog) {
        event.preventDefault()
        search.current?.focus()
      }
    }
    window.addEventListener('keydown', shortcuts)
    return () => window.removeEventListener('keydown', shortcuts)
  }, [dialog])

  useEffect(() => {
    let active = true
    if (!native || !conditions.length) return
    void api.matchTraffic(conditions, anyMatch).then(ids => {
      if (active) setMatches(new Set(ids))
    }).catch(error => {
      if (active) showToast(error instanceof Error ? error.message : String(error), true)
    })
    return () => { active = false }
  }, [rows, conditions, anyMatch, showToast])

  const counts = useMemo(() => {
    const hosts = new Map<string, number>()
    for (const row of rows) hosts.set(row.host, (hosts.get(row.host) ?? 0) + 1)
    return [...hosts.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
  }, [rows])
  const visible = useMemo(() => {
    const terms = query.trim().toLowerCase().split(/\s+/).filter(Boolean)
    const result = rows.filter(row => {
      const matchesFilter = filter === 'all' || (
        filter === 'errors' ? row.status >= 400 || row.error :
        filter === 'json' ? row.contentType.includes('json') :
        filter === row.kind || (filter === 'HTTPS' && row.kind === 'Tunnel')
      )
      const haystack = `${row.method} ${row.url} ${row.status} ${row.kind} ${row.contentType}`.toLowerCase()
      const advancedMatch = !conditions.length || matches?.has(row.id)
      return matchesFilter && advancedMatch && (!host || row.host === host) && terms.every(term => haystack.includes(term))
    })
    result.sort((a, b) => {
      const left = a[sort.key]
      const right = b[sort.key]
      const comparison = typeof left === 'number' && typeof right === 'number'
        ? left - right : String(left).localeCompare(String(right))
      return (sort.reverse ? -comparison : comparison) || b.id - a.id
    })
    return result
  }, [rows, filter, host, query, sort, conditions, matches])
  const activeRow = visible.find(row => row.id === selected) ?? visible[0]
  const activeID = activeRow?.id
  const activePending = activeRow?.pending
  const activeStatus = activeRow?.status
  const activeBreakpoint = activeRow?.breakpoint
  const activeSize = activeRow?.size
  const frameCount = activeRow?.frameCount
  useEffect(() => {
    let active = true
    if (activeID === undefined) return
    void api.detail(activeID).then(value => {
      if (active) setDetail(value)
    }).catch(() => {
      if (active) setDetail(null)
    })
    return () => { active = false }
  }, [activeID, activePending, activeStatus, activeBreakpoint, activeSize, frameCount])
  const activeDetail = detail?.id === activeID ? detail : null
  const errorCount = rows.filter(row => row.status >= 400 || row.error).length
  const address = connectedAddress || `${network.bind}:${port}`
  const completed = visible.filter(row => !row.pending)
  const resetFilters = () => { setQuery(''); setHost(''); setFilter('all'); setSelected(undefined); setSort({ key: 'id', reverse: true }); setConditions([]); setMatches(null) }

  function changeDock(next: Dock) {
    setDock(next)
    localStorage.setItem('peekly.dock', next)
  }

  async function perform(action: () => Promise<void>) {
    setBusy(true)
    try { await action() }
    catch (error) { showToast(error instanceof Error ? error.message : String(error), true) }
    finally { setBusy(false) }
  }

  function selectCaptureMode(mode: CaptureMode | null) {
    setCaptureMode(mode)
    if (mode) localStorage.setItem('peekly.capture-mode', mode)
    else localStorage.removeItem('peekly.capture-mode')
  }

  async function startCapture(mode: CaptureMode) {
    await perform(async () => {
      const endpoint = await api.start(port, mode === 'automatic')
      selectCaptureMode(mode)
      await refresh()
      setDialog(null)
      if (mode === 'automatic' && endpoint !== `${network.bind}:${port}`) {
        showToast(`端口 ${port} 已被占用，系统代理已改用 ${endpoint}`)
      } else {
        showToast(mode === 'automatic' ? '已启用系统代理，请刷新浏览器' : `已启动本地代理 · ${endpoint}`)
      }
    })
  }

  async function toggleCapture() {
    if (!native) { showToast('实时抓包需要 Peekly 桌面应用。', true); return }
    if (!running) {
      if (captureMode) await startCapture(captureMode)
      else openDialog('capture-mode')
      return
    }
    await perform(async () => {
      await api.stop()
      await refresh()
      showToast(systemProxy.active || systemProxy.pending ? '已恢复原系统代理' : '代理已停止，当前记录已保留。')
    })
  }

  function openDialog(next: Dialog) {
    setPortDraft(String(port))
    setRuleDraft(rules.current.join('\n'))
    setToast(undefined)
    setDialog(next)
  }

  async function useResult(result: Detail) {
    resetFilters()
    setSelected(result.id)
    setDetail(result)
    await refresh()
    setDialog(null)
    showToast(`收到响应 · ${result.status || '失败'}`)
  }

  function newRequest(initial: Detail | null) {
    setComposeDraft(initial)
    openDialog('compose')
  }

  function clickedBreakpoint(row: Row) {
    void perform(async () => {
      setBreakHit(await api.detail(row.id))
      openDialog('breakpoint')
    })
  }

  async function copy(value: string) {
    try { await api.copy(value); showToast('已复制到剪贴板') }
    catch (error) { showToast(error instanceof Error ? error.message : String(error), true) }
  }

  return <div className="app">
    <header className="titlebar" style={{ '--wails-draggable': 'drag' } as React.CSSProperties}>
      <div className="wordmark"><img src="./peekly.png" alt="" /><strong>Peekly</strong></div>
      <div className="titlebar-capture" style={{ '--wails-draggable': 'no-drag' } as React.CSSProperties}>
        <button className="btn capture-button" disabled={busy} onClick={() => void toggleCapture()}>
          {running ? <Square size={11} fill="currentColor" /> : <Circle size={10} fill="currentColor" />} {running ? '停止抓包' : '开始抓包'}
        </button>
        <button className="btn tool-button" disabled={!native || busy} title="新建请求" onClick={() => newRequest(null)}><FilePlus2 size={15} /><span>新建请求</span></button>
        <button className="btn tool-button" disabled={!native || busy} title="流量规则" onClick={() => void perform(async () => { setTrafficRules(await api.trafficRules()); openDialog('rules') })}><Waypoints size={15} /><span>流量规则</span></button>
        <button className="btn icon-button" title="导入 HAR" aria-label="导入 HAR" disabled={!native || busy} onClick={() => void perform(async () => {
          const count = await api.importHAR()
          if (count > 0) {
            resetFilters(); await refresh(); showToast(`已导入 ${count} 条请求`)
          }
        })}><ArrowUpFromLine size={15} /></button>
        <button className="btn icon-button" title="导出当前筛选结果为 HAR" aria-label="导出 HAR" disabled={!completed.length} onClick={() => openDialog('export')}><ArrowDownToLine size={15} /></button>
        <button className="btn icon-button" title="清空抓包记录" aria-label="清空抓包记录" disabled={!rows.length || busy} onClick={() => void perform(async () => { await api.clear(); setSelected(undefined); await refresh(); showToast('抓包记录已清空') })}><Trash2 size={15} /></button>
      </div>
      <div className="toolbar-spacer" />
      <div className="titlebar-tools" style={{ '--wails-draggable': 'no-drag' } as React.CSSProperties}>
        <div className="dock-buttons" aria-label="详情面板位置">
          <button className="btn icon-button" title="右侧详情" aria-label="右侧详情" aria-pressed={dock === 'right'} onClick={() => changeDock('right')}><PanelRight size={16} /></button>
          <button className="btn icon-button" title="底部详情" aria-label="底部详情" aria-pressed={dock === 'bottom'} onClick={() => changeDock('bottom')}><PanelBottom size={16} /></button>
        </div>
        <button className="btn icon-button" aria-label={dark ? '切换浅色' : '切换深色'} title={dark ? '切换浅色' : '切换深色'} onClick={() => setAppearance(dark ? 'light' : 'dark')}>{dark ? <Sun size={15} /> : <Moon size={15} />}</button>
        <button className="btn icon-button" title="代理设置" aria-label="代理设置" onClick={() => openDialog('connection')}><Settings2 size={16} /></button>
      </div>
    </header>

    <div className="workspace">
      <nav className="sidebar" aria-label="流量来源">
        <div className="sidebar-top">
          <div className="sidebar-label"><Network size={13} /> 流量来源</div>
          <button className={`source-button fixed-source ${!host ? 'chosen' : ''}`} onClick={() => setHost('')} aria-pressed={!host}>
            <Activity size={14} /><span>所有请求</span><em>{rows.length}</em>
          </button>
          <div className="sidebar-label host-label"><span>主机</span><b className="mono">{counts.length}</b></div>
          <div className="source-list">
            {counts.length === 0 && <p className="no-hosts">等待新的主机连接</p>}
            {counts.map(([name, count]) => <button key={name} title={name} aria-pressed={host === name}
              className={`source-button ${host === name ? 'chosen' : ''}`} onClick={() => setHost(host === name ? '' : name)}>
              {name === 'localhost' || name === '127.0.0.1' ? <Cable size={13} /> : <Globe2 size={13} />}
              <span>{name}</span><em>{count}</em>
            </button>)}
          </div>
        </div>
        <div className="sidebar-bottom">
          <button className="side-action" onClick={() => openDialog('connection')}><CircleHelp size={14} /> 连接指引 <ChevronRight size={12} /></button>
          <button className="side-action" onClick={() => openDialog('tls')}><ShieldCheck size={14} /> HTTPS 解密 {ruleCount ? <em>{ruleCount}</em> : <ChevronRight size={12} />}</button>
        </div>
      </nav>

      <main className="main" aria-label="网络流量">
        <div className="filterbar">
          <div className="filters" aria-label="按协议或状态筛选">
            {filters.map(item => <button key={item.id} aria-pressed={filter === item.id} className={filter === item.id ? 'selected' : ''} onClick={() => setFilter(item.id)}>
              {item.label}{item.id === 'errors' && errorCount > 0 && <span>{errorCount}</span>}
            </button>)}
          </div>
          <button className={`btn tool-button filter-builder ${conditions.length ? 'on' : ''}`} aria-label="高级筛选" title="高级筛选" onClick={() => openDialog('filter')}><ListFilter size={14} /><span>{conditions.length ? `条件 ${conditions.length}` : '高级筛选'}</span></button>
          <label className="input search-field"><Search size={14} /><input ref={search} type="search" aria-label="搜索请求" placeholder="搜索 URL、方法、状态…" value={query} onChange={event => setQuery(event.target.value)} />
            {query ? <button className="clear-query" aria-label="清空搜索" onClick={() => { setQuery(''); search.current?.focus() }}><X size={12} /></button>
              : <kbd className="kbd">{platform === 'darwin' ? <Command size={10} /> : 'Ctrl'} K</kbd>}
          </label>
        </div>

        <div className={`panes dock-${dock}`}>
          <section className="request-list" aria-label="请求列表">
            <div className={`request-scroll ${visible.length ? '' : 'is-empty'}`} role="region" aria-label="流量表格">
              <table>
                <colgroup>{columns.map(column => <col key={column.id} className={`col-${column.id}`} />)}</colgroup>
                <thead><tr>{columns.map(column => <th scope="col" key={column.id} className={`cell-${column.id}`}
                  aria-sort={sort.key === column.id ? (sort.reverse ? 'descending' : 'ascending') : undefined}>
                  <button type="button" onClick={() => setSort({ key: column.id, reverse: sort.key === column.id ? !sort.reverse : ['id', 'started', 'duration', 'size'].includes(column.id) })}>
                    {column.label}{sort.key === column.id && <ArrowDown size={10} className={sort.reverse ? '' : 'flipped'} />}
                  </button>
                </th>)}</tr></thead>
                <tbody>{visible.map((row, index) => <tr key={row.id} className={`request-row ${activeID === row.id ? 'active-row' : ''}`}
                  onClick={event => { setSelected(row.id); if (!(event.target instanceof HTMLElement && event.target.closest('button'))) event.currentTarget.querySelector('button')?.focus({ preventScroll: true }) }}>
                  <td className="mono cell-id">{row.id}</td>
                  <td className="cell-method"><span className={`badge method tone-${methodTone(row.method)}`}>{row.method}</span></td>
                  <td className="cell-path"><button className="request-link" title={row.url} aria-current={activeID === row.id ? 'true' : undefined} onClick={() => setSelected(row.id)}
                    onKeyDown={event => {
                      if (event.key !== 'ArrowUp' && event.key !== 'ArrowDown') return
                      const target = visible[index + (event.key === 'ArrowDown' ? 1 : -1)]
                      if (!target) return
                      event.preventDefault()
                      const currentRow = event.currentTarget.closest('tr')
                      const targetRow = event.key === 'ArrowDown' ? currentRow?.nextElementSibling : currentRow?.previousElementSibling
                      targetRow?.querySelector('button')?.focus()
                      setSelected(target.id)
                    }}>
                    {row.kind === 'Tunnel' || row.kind === 'HTTPS' ? <LockKeyhole size={11} /> : row.kind === 'WebSocket' ? <Zap size={11} /> : <Globe2 size={11} />}
                    <strong className="mono">{row.path}</strong>
                  </button></td>
                  <td className="mono cell-host" title={row.host}>{row.host}</td>
                  <td className="cell-status">{row.breakpoint ? <button className="break-chip" onClick={event => { event.stopPropagation(); setSelected(row.id); clickedBreakpoint(row) }}>已暂停</button> : <Status row={row} />}</td>
                  <td className="mono cell-contentType" title={row.contentType}>{row.kind === 'Tunnel' ? '隧道' : row.kind === 'WebSocket' ? 'WS' : fileType(row.contentType)}</td>
                  <td className="mono cell-started">{formatTime(row.started)}</td>
                  <td className="mono cell-duration">{row.pending && !row.status ? '—' : `${row.duration} ms`}</td>
                  <td className="mono cell-size">{row.pending ? '—' : formatSize(row.size)}</td>
                </tr>)}</tbody>
              </table>
              {!visible.length && <div className="list-empty">
                <div className="empty-icon">{rows.length ? <Search size={21} /> : <Radio size={21} />}</div>
                <h2>{rows.length ? '没有匹配的请求' : running ? '等待网络流量' : '暂无抓包记录'}</h2>
                <p>{rows.length ? '调整搜索条件或筛选器。' : running ? (systemProxy.active ? '刷新浏览器或使用遵循系统代理的应用。' : `将客户端的代理设为 ${address}。`) : '开始抓包后，经过本地代理的请求将在这里显示。'}</p>
                <button className="btn subtle-button" onClick={rows.length ? resetFilters : () => openDialog('connection')}>{rows.length ? '清除筛选' : '连接指引'} <ArrowRight size={13} /></button>
              </div>}
            </div>
          </section>
          <Inspector key={activeID ?? "empty"} row={activeRow} detail={activeDetail} dock={dock} copy={text => void copy(text)}
            saveBody={activeDetail ? request => void perform(async () => { if (await api.saveBody(activeDetail.id, request)) showToast('正文已保存') }) : undefined}
            tools={activeRow && <div className="inspector-tools">
              <button className="btn tool-button" disabled={!activeDetail || activeRow.pending || activeRow.method === 'CONNECT' || !native}
                onClick={() => newRequest(activeDetail)}><PenLine size={13} /> 编辑重发</button>
              <button className="btn tool-button" disabled={!activeDetail || activeRow.pending || activeRow.method === 'CONNECT' || activeRow.kind === 'WebSocket' || activeDetail.requestTruncated || !native || busy}
                onClick={() => void perform(async () => { if (activeRow) await useResult(await api.repeat(activeRow.id)) })}><Repeat2 size={13} /> 重发</button>
              <button className="btn tool-button" disabled={!activeDetail || activeRow.pending || rows.length < 2} onClick={() => {
                setComparison(activeDetail); openDialog('compare')
              }}><GitCompareArrows size={13} /> 对比</button>
              {activeRow.breakpoint && <button className="btn tool-button on" onClick={() => clickedBreakpoint(activeRow)}>断点操作</button>}
            </div>} />
        </div>
      </main>
    </div>

    <footer className="statusbar">
      <span className="statusbar-mode"><i className={running ? 'on' : ''} />{systemProxy.active ? '系统代理已接入' : running ? '正在监听' : '监听已停止'}</span>
      <span className="foot-divider"></span><button className="mono" title="复制代理地址" onClick={() => void copy(address)}>{address}</button>
      {systemProxy.pending && <button className="desktop-error" disabled={busy} onClick={() => void perform(async () => {
        await api.restoreSystemProxy(); await refresh(); showToast('已恢复原系统代理')
      })}>恢复系统代理</button>}
      {desktopProblem && <span className="desktop-error" title={desktopProblem}>桌面连接异常</span>}
      <div className="footer-end"><span>{visible.length === rows.length ? `${visible.length} 条请求` : `${visible.length} / ${rows.length} 条请求`}</span><span className="foot-divider"></span><span className={errorCount ? 'footer-error' : ''}>{errorCount} 错误</span><span className="foot-divider"></span><span className="mono">v0.2.2</span></div>
    </footer>
    {toast && !dialog && <div className={`toast-bar ${toast.error ? 'toast-error' : ''}`} role={toast.error ? 'alert' : 'status'}>{toast.error ? <CircleHelp size={17} /> : <Check size={17} />}{toast.message}</div>}
    {dialog === 'capture-mode' && <Modal title="开始抓包" icon={<Circle size={18} />} notice={toast} onClose={() => setDialog(null)}>
      <div className="dialog-section capture-mode-choices">
        <p>Peekly 可以临时设置系统代理。浏览器等遵循系统设置的应用会自动接入，停止抓包时恢复原配置。</p>
        {platform === 'darwin' && <p>macOS 可能会请求管理员授权。</p>}
        <p>可在连接指引中更改抓包方式。</p>
      </div>
      <div className="modal-actions"><button className="btn subtle-button" disabled={busy} onClick={() => void startCapture('manual')}>手动连接</button>
        <button className="btn capture-button" disabled={busy} onClick={() => void startCapture('automatic')}>使用系统代理</button></div>
    </Modal>}
    {dialog === 'connection' && <Modal wide title="连接指引" icon={<Cable size={20} />} notice={toast} onClose={() => setDialog(null)}>
      <ConnectionGuide address={address} host={network.bind} running={running} platform={platform} portDraft={portDraft}
        setPortDraft={setPortDraft} savedPort={port} automaticActive={systemProxy.active} captureMode={captureMode} setCaptureMode={selectCaptureMode} appearance={appearance} setAppearance={setAppearance} copy={value => void copy(value)}
        savePort={() => {
          const next = Number(portDraft)
          if (!/^\d{4,5}$/.test(portDraft) || next < 1024 || next > 65535) { showToast('端口需在 1024–65535 之间。', true); return }
          setPort(next)
          showToast('代理端口已更新')
        }} />
      <div className="modal-actions guide-actions">
        <button className="btn subtle-button" disabled={running || busy || !native} onClick={() => void perform(async () => {
          const [settings, addresses] = await Promise.all([api.network(), api.localAddresses()])
          setNetwork(settings); setNetworkAddresses(addresses); openDialog('network')
        })}><Network size={14} /> 网络设置</button>
        <button className="btn subtle-button" onClick={() => openDialog('tls')}><LockKeyhole size={14} /> HTTPS 解密</button>
        <button className="btn capture-button close-guide" onClick={() => setDialog(null)}>完成</button>
      </div>
    </Modal>}
    {dialog === 'tls' && <Modal title="HTTPS 解密" icon={<ShieldCheck size={21} />} notice={toast} onClose={() => setDialog(null)}>
      <div className="dialog-section">
        <h3>信任本地根证书</h3>
        <p>{platform === 'windows' ? '导出证书后，将它导入当前用户的「受信任的根证书颁发机构」。' : '导出证书后，在「钥匙串访问」中导入，并在证书的「信任」设置中勾选始终信任 SSL。'}</p>
        <div className="form-row"><button className="btn subtle-button" disabled={busy || !native} onClick={() => void perform(async () => {
          if (await api.exportCertificate()) showToast('根证书已导出')
        })}><ArrowDownToLine size={15} /> 导出根证书</button>
          <button className="btn subtle-button" disabled={busy || !native} onClick={() => void perform(async () => setFingerprint((await api.certificate()).fingerprint))}>显示 SHA-256 指纹</button></div>
        {fingerprint && <button className="command-block fingerprint mono" onClick={() => void copy(fingerprint)}><code>{fingerprint}</code><Copy size={15} /></button>}
      </div>
      <div className="dialog-section"><label className="field-label" htmlFor="tls-hosts">需要解密的域名</label>
        <p>一行一个域名、IP 或子域名通配符。设置立即用于新连接，每次启动需重新开启。</p>
        <textarea autoFocus className="textarea text-input mono tls-rules" id="tls-hosts" value={ruleDraft} placeholder={'api.example.com\n*.dev.example.com'} onChange={event => setRuleDraft(event.target.value)} />
      </div>
      <div className="modal-actions"><button className="btn subtle-button" onClick={() => setDialog(null)}>取消</button>
        <button className="btn capture-button" disabled={busy || !native} onClick={() => void perform(async () => {
          const next = await api.rules(ruleDraft.split('\n'))
          rules.current = next
          setRuleCount(next.length)
          setDialog(null)
          showToast(`已保存 ${next.length} 条 HTTPS 解密规则`)
        })}>应用规则</button></div>
    </Modal>}
    {dialog === 'export' && <Modal title="导出 HAR" icon={<ArrowDownToLine size={21} />} notice={toast} onClose={() => setDialog(null)}>
      <div className="dialog-section"><p>导出当前筛选下的 {completed.length} 条已完成请求。文件包含请求路径和时间。</p><label className="inline-label"><input type="checkbox" checked={redacted} onChange={event => setRedacted(event.target.checked)} /> 隐藏查询值和敏感标头，省略正文</label></div>
      <div className="modal-actions"><button className="btn subtle-button" onClick={() => setDialog(null)}>取消</button><button className="btn capture-button" disabled={busy || !completed.length} onClick={() => void perform(async () => {
        if (await api.exportHAR(completed.map(row => row.id), redacted)) { showToast('HAR 文件已保存'); setDialog(null) }
      })}>保存 HAR</button></div>
    </Modal>}
    {dialog === 'compose' && <Modal wide title={composeDraft ? '编辑并重发' : '新建 HTTP 请求'} icon={<FilePlus2 size={20} />} notice={toast} onClose={() => setDialog(null)}>
      <ComposePanel initial={composeDraft} pending={busy} close={() => setDialog(null)} submit={(input: () => RequestInput) => void perform(async () => {
        const req = input()
        await useResult(await api.compose(req))
      })} />
    </Modal>}
    {dialog === 'network' && <Modal wide title="网络设置" icon={<Network size={20} />} notice={toast} onClose={() => setDialog(null)}>
      <NetworkPanel initial={network} addresses={networkAddresses} pending={busy} close={() => setDialog(null)} submit={factory => void perform(async () => {
        const options = factory()
        await api.saveNetwork(options); setNetwork(options); setDialog(null); showToast('网络设置已应用')
      })} />
    </Modal>}
    {dialog === 'rules' && <Modal wide title="流量规则" icon={<Waypoints size={20} />} notice={toast} onClose={() => setDialog(null)}>
      <RulesPanel initial={trafficRules} pending={busy} close={() => setDialog(null)} submit={items => void perform(async () => {
        await api.saveTrafficRules(items); setTrafficRules(items); setDialog(null); showToast(`已应用 ${items.filter(item => item.enabled).length} 条流量规则`)
      })} />
    </Modal>}
    {dialog === 'filter' && <Modal wide title="高级筛选" icon={<ListFilter size={20} />} notice={toast} onClose={() => setDialog(null)}>
      <FilterPanel initial={conditions} any={anyMatch} close={() => setDialog(null)} submit={(items, any) => void perform(async () => {
        for (const item of items) if (item.op === 'regex') new RegExp(item.value)
        setMatches(items.length && native ? new Set(await api.matchTraffic(items, any)) : null)
        setConditions(items); setAnyMatch(any); setDialog(null)
      })} />
    </Modal>}
    {dialog === 'compare' && comparison && <Modal wide title="比较请求" icon={<GitCompareArrows size={20} />} notice={toast} onClose={() => setDialog(null)}>
      <DiffPanel base={comparison} rows={rows} fetchDetail={api.detail} />
      <div className="modal-actions"><button className="btn subtle-button" onClick={() => setDialog(null)}>关闭</button></div>
    </Modal>}
    {dialog === 'breakpoint' && breakHit && <Modal wide title={breakHit.breakpoint === 'response' ? '响应断点' : '请求断点'} icon={<Waypoints size={20} />} notice={toast} onClose={() => setDialog(null)}>
      <BreakpointPanel detail={breakHit} pending={busy} close={() => setDialog(null)} submit={(action: () => BreakpointAction) => void perform(async () => {
        await api.resolveBreakpoint(breakHit.id, action()); setDialog(null); await refresh()
      })} />
    </Modal>}
  </div>
}
