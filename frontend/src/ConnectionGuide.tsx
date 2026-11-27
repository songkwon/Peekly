import { useState } from 'react'
import { ArrowRight, Check, Clipboard, Globe2, Monitor, Radio, Terminal } from 'lucide-react'

export function ConnectionGuide({ address, host, running, platform, portDraft, setPortDraft, savePort, savedPort, automaticActive, captureMode, setCaptureMode, appearance, setAppearance, copy }: {
  address: string, host: string, running: boolean, platform: string, portDraft: string, setPortDraft: (value: string) => void,
  savePort: () => void, savedPort: number, automaticActive: boolean, captureMode: 'automatic' | 'manual' | null,
  setCaptureMode: (mode: 'automatic' | 'manual' | null) => void, appearance: 'system' | 'light' | 'dark',
  setAppearance: (value: 'system' | 'light' | 'dark') => void, copy: (value: string) => void
}) {
  const [guide, setGuide] = useState<'system' | 'terminal' | 'device'>('system')
  const windows = platform === 'windows'
  const listeningPort = running ? Number(address.slice(address.lastIndexOf(':') + 1)) : savedPort
  const usingAlternatePort = running && listeningPort !== savedPort
  const command = `${windows ? 'curl.exe' : 'curl'} --proxy http://${address} http://example.com/`
  const guides = [{ id: 'system', title: '系统代理', icon: Monitor }, { id: 'terminal', title: '终端', icon: Terminal }, { id: 'device', title: '其他设备', icon: Globe2 }] as const
  return <div className="guide-content">
    <div className="guide-endpoint">
      <div className="guide-endpoint-icon"><Radio size={20} /></div>
      <div className="guide-endpoint-address"><small>HTTP 代理地址</small><strong className="mono">{address}</strong></div>
      <span className={`guide-state ${running ? 'running' : ''}`}><i />{running ? '正在监听' : '未启动'}</span>
      <button className="btn subtle-button" onClick={() => copy(address)}><Clipboard size={14} /> 复制地址</button>
    </div>

    <div className="guide-layout">
      <div className="guide-method">
        <div className="guide-tabs" aria-label="连接方式">
          {guides.map(({ id, title, icon: Icon }) => <button type="button" key={id} aria-pressed={guide === id} onClick={() => setGuide(id)}><Icon size={14} /> {title}</button>)}
        </div>
        {guide === 'system' && automaticActive ? <div className="guide-copy">
          <strong>系统代理已接入 Peekly</strong><p>刷新浏览器即可看到新请求。停止抓包会恢复启动前的代理设置。</p>
        </div> : guide === 'system' && captureMode !== 'manual' ? <div className="guide-copy">
          <strong>自动接入浏览器等应用</strong><p>点击「开始抓包」，选择「使用系统代理」，再刷新浏览器。停止抓包会恢复原系统设置。</p>
          <p>若目标应用使用自己的代理设置，可在该应用中填写上方代理地址。</p>
        </div> : guide === 'system' ? <ol className="guide-steps">
          <li><span>{running ? <Check size={12} /> : '1'}</span><div><strong>启动抓包</strong><p>在工具栏选择「开始抓包」。</p></div></li>
          <li><span>2</span><div><strong>打开{windows ? ' Windows ' : ' macOS '}代理设置</strong>
            <p>{windows ? '设置 → 网络和 Internet → 代理 → 手动设置代理' : '系统设置 → 网络 → 当前网络 → 详细信息 → 代理'}</p></div></li>
          <li><span>3</span><div><strong>填写代理地址</strong>
            <p>{windows ? `将服务器设为 ${host}，端口设为 ${savedPort}。` : `开启 HTTP 与 HTTPS 网页代理，服务地址设为 ${host}，端口设为 ${savedPort}。`}</p></div></li>
        </ol> : guide === 'terminal' ? <div className="guide-copy">
          <strong>发送一条测试请求</strong><p>启动抓包后，在终端执行此命令。请求会出现在流量列表。</p>
          <button className="command-block mono" onClick={() => copy(command)}><code>{command}</code><Clipboard size={14} /></button>
        </div> : <div className="guide-copy">
          <strong>从同一局域网连接</strong><p>在「网络设置」选择本机局域网地址，并将设备 IP 加入允许列表。</p>
          <p>在设备的 Wi-Fi 代理设置中填入上方地址。查看 HTTPS 正文还需要在设备上信任 Peekly 根证书。</p>
        </div>}
        {guide === 'system' && !automaticActive && captureMode === 'manual' && <p className="guide-footnote"><ArrowRight size={12} /> 抓包结束后，撤销为 Peekly 添加的系统代理设置。</p>}
      </div>

      <div className="guide-settings">
        <h3>应用设置</h3>
        <label htmlFor="proxy-port">监听端口</label>
        <div className="guide-input-row">
          <input className="input text-input mono" type="text" inputMode="numeric" pattern="[0-9]{4,5}" maxLength={5} id="proxy-port" disabled={running} value={portDraft} onChange={event => setPortDraft(event.target.value.replace(/\D/g, ''))} onKeyDown={event => {
            if (event.key === 'Enter' && portDraft !== String(savedPort)) { event.preventDefault(); savePort() }
          }} />
          <button className="btn subtle-button" disabled={running || portDraft === String(savedPort)} onClick={savePort}>保存</button>
        </div>
        {running && <small>{usingAlternatePort ? `首选端口被占用，本次使用 ${listeningPort}。` : '停止抓包后可修改端口'}</small>}
        <label htmlFor="capture-mode">抓包方式</label>
        <select className="text-input" id="capture-mode" disabled={running} value={captureMode ?? 'ask'} onChange={event => {
          const mode = event.target.value
          setCaptureMode(mode === 'automatic' || mode === 'manual' ? mode : null)
        }}>
          <option value="ask">启动时询问</option><option value="automatic">自动配置系统代理</option><option value="manual">手动连接</option>
        </select>
        <label htmlFor="appearance">外观</label>
        <select className="text-input" id="appearance" value={appearance} onChange={event => setAppearance(event.target.value as typeof appearance)}>
          <option value="system">跟随系统</option><option value="light">浅色</option><option value="dark">深色</option>
        </select>
      </div>
    </div>
  </div>
}
